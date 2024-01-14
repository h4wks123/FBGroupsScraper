package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"path"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
)

const (
	endpoint   = `https://www.facebook.com/groups`
	groupID    = `900072927547214`
	timeout    = 15
	maxRetries = 5
)

type Post struct {
	id      int64
	content string
	images  []string
}

func main() {
	// TODO: Priority 4: Add a way to pass the following as command line arguments
	// Rate limit to limit the number of parses per second
	// Post size to limit the number of posts
	// Output folder to save the images to
	// Filenames for the csvs
	// Make it optional to scrape comments and attachments

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
	if err := chromedp.Run(ctx, trackNodeStability(feed.FullXPath(), "feed", time.Second)); err != nil {
		log.Fatal(err)
	}

	log.Println("Retrieving posts...")
	var retries int
	var postsScraped int64
	var postNodes, imageNodes []*cdp.Node
	data := Post{images: make([]string, 0)}
	for i := 0; i < 10; i++ {
		if err := chromedp.Run(ctx,
			// Wait for the feed to become stable
			chromedp.Poll(`window.stable.feed.value`, nil, chromedp.WithPollingTimeout(timeout*time.Second)),
			// Expand all content
			chromedp.Evaluate(`document.querySelectorAll('[data-ad-preview="message"] div:last-child[role="button"]').forEach((n)=> n.click())`, nil),
			// Retrieve the posts that have images
			runTasksWithTimeout(5*time.Second, chromedp.Tasks{
				chromedp.Nodes(`[aria-posinset][role="article"] div:nth-child(3):has(img[src*="fna.fbcdn.net"])`, &postNodes, chromedp.ByQueryAll, chromedp.FromNode(feed)),
			}),
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

		// thus the only thing I need are the images, the locations, and optionally the name of the snakes

		// to get HD images, we need to source the photo links NOT the image links, and then wait for the page to load
		// var content string

		// create a channel, to receive the image links, and then delegate it to goroutines, so it runs in the background

		for _, post := range postNodes {
			postsScraped = postsScraped + 1
			data.id = postsScraped

			if err := chromedp.Run(ctx, chromedp.Nodes(`img[src*="fna.fbcdn.net"]`, &imageNodes, chromedp.ByQueryAll, chromedp.FromNode(post))); err != nil {
				log.Fatal(err)
			}

			if err := chromedp.Run(ctx, getAllTextContentInNode(post.FullXPath(), &data.content)); err != nil {
				log.Fatal(err)
			}

			data.images = data.images[:0]
			for _, img := range imageNodes {
				imgUrl, err := url.Parse(img.AttributeValue("src"))
				if err != nil {
					log.Print(err)
					continue
				}

				_, filename := path.Split(imgUrl.Path)
				data.images = append(data.images, filename)
			}

			fmt.Printf("%+v\n", data)
		}

		// Reset the retries
		retries = 0
		// Remove the posts that have been processed
		if err := chromedp.Run(ctx,
			chromedp.Evaluate(`document.querySelectorAll('[data-pagelet="GroupFeed"] > [role="feed"] > div:nth-last-child(n+4)').forEach((n) => n.remove());`, nil),
		); err != nil {
			log.Fatal(err)
		}
	}
}

func getAllTextContentInNode(xpath string, contentRef *string) chromedp.Action {
	return chromedp.Evaluate(fmt.Sprintf(`
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
    `, xpath), &contentRef)
}

func trackNodeStability(xpath, label string, debounce time.Duration) chromedp.Action {
	return chromedp.Evaluate(fmt.Sprintf(`
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
    `, xpath, label, label, label, label, label, label, label, debounce.Milliseconds(), label), nil)
}

func runTasksWithTimeout(timeout time.Duration, tasks chromedp.Tasks) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		return tasks.Do(timeoutCtx)
	}
}

// add the cookie script here
func getFacebookGroupFeed(c context.Context, groupID string) (*cdp.Node, error) {
	var nodes []*cdp.Node
	tasks := chromedp.Tasks{
		// Navigate to page
		chromedp.Navigate(fmt.Sprintf("%s/%s/", endpoint, groupID)),
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
