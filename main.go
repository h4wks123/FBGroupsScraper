package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/LaplaceXD/FBGroupsScraper/models"
	"github.com/LaplaceXD/FBGroupsScraper/scripts"
	"github.com/LaplaceXD/FBGroupsScraper/workers"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
)

const (
	endpoint        = `https://www.facebook.com/groups`
	postsFile       = "posts.csv"
	attachmentsFile = "attachments.csv"
	imagesDir       = "images"
)

var (
	locationRegex    = regexp.MustCompile(`(?i)loc(?:ation)?(?:.*?)([^ation]\w[\w\ \,]+)`)
	BypassFailed     = errors.New("error: unable to bypass redirect")
	UnableToRetrieve = errors.New("error: unable to retrieve")
	groupID          string
	outputDir        string
	headless         bool
	timeout          int
	maxRetries       int
	maxPosts         int
)

func init() {
	flag.StringVar(&groupID, "groupID", "900072927547214", "Facebook Group ID to scrape")
	flag.StringVar(&outputDir, "output", "results", "Output directory for scraped data")
	flag.BoolVar(&headless, "headless", false, "Run in headless mode")
	flag.IntVar(&timeout, "timeout", 5, "Timeout for Each Post Scrape")
	flag.IntVar(&maxRetries, "retries", 5, "Maximum number of retries for page scrape before giving up")
	flag.IntVar(&maxPosts, "posts", 10, "Maximum number of posts to scrape (may be higher than this)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n", os.Args[0])
		flag.PrintDefaults()
	}
}

func main() {
	flag.Parse()

	if err := os.MkdirAll(filepath.Join(outputDir, imagesDir), os.ModePerm); err != nil {
		log.Fatal(err)
	}

	// Instead wait group, and channels for concurrency
	var wg sync.WaitGroup

	imgDownloadChan := make(chan workers.Downloader, 128)
	postChan := make(chan workers.CSVWriter, 64)
	attachmentsChan := make(chan workers.CSVWriter, 64)

	// Create a download client, this ensures that TLS connections are reused
	client := &http.Client{
		Transport: &http.Transport{
			// There are MaxWorkers - 2 download workers, we double it for margin
			MaxIdleConnsPerHost: 2 * (workers.MaxWorkers - 2),
			IdleConnTimeout:     2 * time.Minute,
		},
	}

	wg.Add(workers.MaxWorkers)
	for i := 1; i <= workers.MaxWorkers-2; i++ {
		go workers.FileDownload(workers.LogOnError, &wg, imgDownloadChan, client, func(d workers.Downloader) string {
			return filepath.Join(outputDir, imagesDir, d.(models.Image).Name)
		})
	}
	go workers.CSVWrite(workers.LogOnError, &wg, postChan, filepath.Join(outputDir, postsFile), []string{"post_id", "location", "content"})
	go workers.CSVWrite(workers.LogOnError, &wg, attachmentsChan, filepath.Join(outputDir, attachmentsFile), []string{"post_id", "image"})

	// Open Browser
	browser, cancel := chromedp.NewExecAllocator(context.Background(),
		append(chromedp.DefaultExecAllocatorOptions[:], chromedp.Flag("headless", headless))...,
	)

	ctx, cancel := chromedp.NewContext(browser)
	defer cancel()

	// Retrieve facebook group feed
	log.Println("Retrieving facebook group feed...")
	feed, err := getFacebookGroupFeed(ctx, groupID)
	if err != nil {
		log.Fatal(err)
	}

	// Load scripts, and track feed for stability used for polling it for new loaded posts
	log.Println("Tracking facebook group feed...")
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(strings.Join(scripts.All(), "\n\n"), nil),
		chromedp.Evaluate(scripts.TrackStability(feed.FullXPath(), "feed", 2*time.Second), nil),
	); err != nil {
		log.Fatal(err)
	}

	// Main Scraping Logic
	log.Println("Scraping facebook group feed...")
	var retries, postsScraped int
	var postNodes []*cdp.Node
	for postsScraped < maxPosts && retries < maxRetries {
		if err := chromedp.Run(ctx,
			// Let the page load naturally first
			chromedp.Sleep(3*time.Second),
			// Then, check if the feed is stable (i.e., no more changes have occurred to its children)
			chromedp.Poll(scripts.CheckStability("feed"), nil, chromedp.WithPollingInterval(time.Second)),
			// Expand all See More... Content
			chromedp.Evaluate(`document.querySelectorAll('[data-ad-preview="message"] div:last-child[role="button"]').forEach((n)=> n.click())`, nil),
			// Retrieve the posts that have images on a timeout
			chromedp.ActionFunc(func(ctx context.Context) error {
				timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
				defer cancel()

				return chromedp.Nodes(
					`[aria-posinset][role="article"] div:not([class]):nth-child(3):has(a:is([href*="set=pcb."], [href*="set=gm."]) img[src*="fna.fbcdn.net"])`,
					&postNodes,
					chromedp.ByQueryAll,
					chromedp.FromNode(feed),
				).Do(timeoutCtx)
			}),
		); err != nil {
			if err == context.DeadlineExceeded {
				fmt.Printf("Scraping timed out. Retrying (%d / %d)...", retries, maxRetries)
				retries += 1
				if err := chromedp.Run(ctx, chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight);`, nil)); err != nil {
					log.Fatal(err)
				}

				continue
			}

			log.Fatal(err)
		}

		postsScraped += len(postNodes)
		for _, postNode := range postNodes {
			post, err := extractPost(postNode, ctx)
			if err != nil {
				postsScraped -= 1
				continue
			}

			postChan <- post
			attachmentsChan <- models.Attachments{ID: post.ID, Images: post.Images}

			for _, img := range post.Images {
				imgDownloadChan <- img
			}
		}

		// Remove the posts that have been processed
		if err := chromedp.Run(ctx,
			chromedp.Evaluate(`document.querySelectorAll('[data-pagelet="GroupFeed"] > [role="feed"] > div:nth-last-child(n+4)').forEach((n) => n.remove());`, nil),
		); err != nil {
			log.Fatal(err)
		}

		retries = 0
		log.Printf("Scraped %d posts...\n", postsScraped)
		postNodes = postNodes[:0]
	}

	if postsScraped == maxPosts {
		log.Println("Max retries reached.")
	}

	// Close channels and wait for the workers to finish
	log.Println("Waiting for other threads to finish...")
	close(imgDownloadChan)
	close(postChan)
	close(attachmentsChan)
	wg.Wait()

	log.Printf("Done! %d posts scraped.", postsScraped)
}

func extractPost(postNode *cdp.Node, ctx context.Context) (*models.Post, error) {
	var aNodes, imageNodes []*cdp.Node
	var content, location string

	if err := chromedp.Run(ctx,
		// Get Post Content
		chromedp.Evaluate(scripts.ExtractAllText(postNode.FullXPath(), "[data-ad-preview='message']"), &content),
		// Get Post Image Links
		chromedp.Nodes(`a[role="link"]:is([href*="set=pcb."], [href*="set=gm."]):has(img)`, &aNodes, chromedp.ByQueryAll, chromedp.FromNode(postNode)),
		// Get Post Images
		chromedp.Nodes(`img[src*="fna.fbcdn.net"]`, &imageNodes, chromedp.ByQueryAll, chromedp.FromNode(postNode)),
	); err != nil {
		return nil, fmt.Errorf("post: %s\n", err.Error())
	}

	postUrl, err := url.Parse(aNodes[0].AttributeValue("href"))
	if err != nil {
		return nil, fmt.Errorf("post: %s\n", err.Error())
	}

	if strings.Contains(strings.ToLower(content), "loc") {
		for _, line := range strings.Split(content, "\n") {
			if match := locationRegex.FindStringSubmatch(line); len(match) > 1 {
				location = strings.TrimSpace(match[1])
				break
			}
		}
	}

	post := &models.Post{
		ID:       strings.TrimPrefix(strings.TrimPrefix(postUrl.Query().Get("set"), "pcb."), "gm."),
		Location: location,
		Content:  content,
		Images:   make([]models.Image, 0, len(imageNodes)),
	}

	for _, img := range imageNodes {
		imgUrl, err := url.Parse(img.AttributeValue("src"))
		if err != nil {
			continue
		}

		post.Images = append(post.Images, models.Image{
			Name: filepath.Base(imgUrl.Path),
			Url:  img.AttributeValue("src"),
		})
	}

	return post, nil
}

func navigateWithBypass(url string, sleep time.Duration, retries int) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		var navigatedUrl string

		if err := chromedp.Run(ctx,
			chromedp.Navigate(url),
			chromedp.Location(&navigatedUrl),
		); err != nil {
			return err
		}

		for i := 0; navigatedUrl != url && i < retries; i++ {
			if err := chromedp.Run(ctx,
				chromedp.Sleep(sleep),
				chromedp.Navigate(url),
				chromedp.Location(&navigatedUrl),
			); err != nil {
				return err
			}
		}

		if navigatedUrl != url {
			return fmt.Errorf("login page: %s\n", BypassFailed.Error())

		}

		return nil
	}
}

func getFacebookGroupFeed(ctx context.Context, groupID string) (*cdp.Node, error) {
	var nodes []*cdp.Node

	if err := chromedp.Run(ctx,
		// Bypass Facebook's redirect to the Login page
		navigateWithBypass(fmt.Sprintf("%s/%s/", endpoint, groupID), 5*time.Second, 3),
		// Wait for the login popup to appear
		chromedp.WaitVisible(`#login_popup_cta_form`, chromedp.ByQuery),
		// Close the login popup
		chromedp.Click(`[role="dialog"] > div > [role="button"]`, chromedp.ByQuery, chromedp.NodeVisible),
		// Retrieve the feed
		chromedp.Nodes(`[data-pagelet="GroupFeed"] > [role="feed"]`, &nodes, chromedp.ByQuery),
		// Removes a buffer element at the top of the feed, which is not really used
		chromedp.Evaluate(`document.querySelector('[data-pagelet="GroupFeed"] > [role="feed"] > div:first-child')?.remove();`, nil),
		// Move the page to render the post breakpoint for loading new set of posts
		chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight);`, nil),
	); err != nil {
		return nil, err
	} else if len(nodes) == 0 {
		return nil, fmt.Errorf("feed %s: %s\n", groupID, UnableToRetrieve.Error())
	}

	return nodes[0], nil
}
