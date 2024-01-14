package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
)

const (
	endpoint   = `https://www.facebook.com/groups`
	groupID    = `900072927547214`
	timeout    = 15
	maxRetries = 5
	maxPosts   = 10 // this is just a min bound, the actual number of posts scraped may be higher than this
)

var locationRegex = regexp.MustCompile(`(?i)loc(?:ation)?(?:.*?)([^ation]\w[\w\ \,]+)`)

type Post struct {
	ID       string
	Location string
	Content  string
	Images   []string
}

func main() {
	// TODO: Priority 4: Add a way to pass the following as command line arguments
	// Rate limit to limit the number of parses per second
	// Output folder to save the images to
	// Filenames for the csvs

	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.Flag("headless", false))
	browserCtx, closeBrowser := chromedp.NewExecAllocator(context.Background(), opts...)
	defer closeBrowser()

	ctx, cancel := chromedp.NewContext(browserCtx)
	defer cancel()

	log.Println("Retrieving facebook group...")
	feed, err := getFacebookGroupFeed(ctx, groupID)
	if err != nil {
		log.Fatal(err)
	}

	// Track the feed for stability (i.e., no changes have occured to its children after a period of time)
	if err := chromedp.Run(ctx, chromedp.Evaluate(trackNodeStabilityJS(feed.FullXPath(), "feed", time.Second), nil)); err != nil {
		log.Fatal(err)
	}

	log.Println("Retrieving posts...")
	var retries, postsScraped int
	var postNodes []*cdp.Node
	for postsScraped <= maxPosts {
		if err := chromedp.Run(ctx,
			// Wait for the feed to become stable
			chromedp.Poll(`window.stable.feed.value`, nil, chromedp.WithPollingTimeout(timeout*time.Second)),
			// Expand all content
			chromedp.Evaluate(`document.querySelectorAll('[data-ad-preview="message"] div:last-child[role="button"]').forEach((n)=> n.click())`, nil),
			// Retrieve the posts that have images
			runTasksWithTimeout(5*time.Second,
				chromedp.Nodes(`[aria-posinset][role="article"] div:nth-child(3):has(img[src*="fna.fbcdn.net"])`, &postNodes, chromedp.ByQueryAll, chromedp.FromNode(feed)),
			),
		); err != nil {
			if err != context.DeadlineExceeded || retries >= maxRetries {
				log.Fatal(err)
			}

			retries = retries + 1
			if err := chromedp.Run(ctx, chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight);`, nil)); err != nil {
				log.Fatal(err)
			}

			continue
		}

		// create a channel, to receive the image links, and then delegate it to goroutines, so it runs in the background
		postsScraped = postsScraped + len(postNodes)
		for _, postNode := range postNodes {
			post, err := extractPost(postNode, ctx)
			if err != nil {
				postsScraped -= 1
				log.Println(err)
				continue
			}

			log.Printf("%+v\n", post)
		}

		retries = 0
		if err := chromedp.Run(ctx,
			// Remove the posts that have been processed
			chromedp.Evaluate(`document.querySelectorAll('[data-pagelet="GroupFeed"] > [role="feed"] > div:nth-last-child(n+4)').forEach((n) => n.remove());`, nil),
		); err != nil {
			log.Fatal(err)
		}
	}
}

func extractPost(postNode *cdp.Node, ctx context.Context) (*Post, error) {
	var aNodes, imageNodes []*cdp.Node
	var content, location string

	if err := chromedp.Run(ctx,
		// Get Post Content
		chromedp.Evaluate(extractPostContentJS(postNode.FullXPath()), &content),
		// Get Post Image Links
		chromedp.Nodes(`a[role="link"]`, &aNodes, chromedp.ByQueryAll, chromedp.FromNode(postNode)),
		// Get Post Images
		chromedp.Nodes(`img[src*="fna.fbcdn.net"]`, &imageNodes, chromedp.ByQueryAll, chromedp.FromNode(postNode)),
	); err != nil {
		return nil, err
	}

	postUrl, err := url.Parse(aNodes[0].AttributeValue("href"))
	if err != nil {
		return nil, err
	}

	if strings.Contains(strings.ToLower(content), "loc") {
		for _, line := range strings.Split(content, "\n") {
			if match := locationRegex.FindStringSubmatch(line); len(match) > 1 {
				location = strings.TrimSpace(match[1])
				break
			}
		}
	}

	post := &Post{
		ID:       strings.TrimPrefix(strings.TrimPrefix(postUrl.Query().Get("set"), "pcb."), "gm."),
		Location: location,
		Content:  content,
		Images:   make([]string, 0, len(imageNodes)),
	}

	for _, img := range imageNodes {
		imgUrl, err := url.Parse(img.AttributeValue("src"))
		if err != nil {
			continue
		}

		_, filename := path.Split(imgUrl.Path)
		post.Images = append(post.Images, filename)
	}

	return post, nil
}

func extractPostContentJS(xpath string) string {
	return fmt.Sprintf(`
        (function() {
            let text = "";
            const content = document
                .evaluate("%s", document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null)
                ?.singleNodeValue
                ?.querySelector("[data-ad-preview='message']");

            if (content) {
                const walker = document.createTreeWalker(content, NodeFilter.SHOW_TEXT); 
                while(walker.nextNode()) text += walker.currentNode.textContent + "\n";
            }

            return text
        })()
    `, xpath)
}

func trackNodeStabilityJS(xpath, label string, debounce time.Duration) string {
	return fmt.Sprintf(`
        window.stable = window.stable || {};
        let node = document.evaluate("%s", document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null)?.singleNodeValue;

        if(node) {
            window.stable["%s"] = window.stable["%s"] || {};
            window.stable["%s"].observer = new MutationObserver(function() {
                window.stable["%s"].value = false;

                clearTimeout(window.stable["%s"].timeout);
                window.stable["%s"].timeout = setTimeout(() => window.stable["%s"].value = true, %d);
            });

            window.stable["%s"].observer.observe(node, { childList: true });
        }
    `, xpath, label, label, label, label, label, label, label, debounce.Milliseconds(), label)
}

func runTasksWithTimeout(timeout time.Duration, tasks ...chromedp.Action) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		return chromedp.Tasks(tasks).Do(timeoutCtx)
	}
}

func navigateWithBypass(url string, sleep time.Duration, retries int) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		var navigatedUrl string

		err := chromedp.Tasks{
			chromedp.Navigate(url),
			chromedp.Location(&navigatedUrl),
		}.Do(ctx)
		if err != nil {
			return err
		}

		for i := 0; navigatedUrl != url && i < retries; i++ {
			err := chromedp.Tasks{
				chromedp.Sleep(sleep),
				chromedp.Navigate(url),
				chromedp.Location(&navigatedUrl),
			}.Do(ctx)
			if err != nil {
				return err
			}
		}

		if navigatedUrl != url {
			return fmt.Errorf("error: unable to bypass redirect to login page")
		}

		return nil
	}
}

func getFacebookGroupFeed(c context.Context, groupID string) (*cdp.Node, error) {
	var nodes []*cdp.Node

	tasks := chromedp.Tasks{
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
	}

	if err := chromedp.Run(c, tasks...); err != nil {
		return nil, err
	} else if len(nodes) == 0 {
		return nil, fmt.Errorf("error: unable to extract feed")
	}

	return nodes[0], nil
}
