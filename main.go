package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
)

const (
	endpoint = `https://www.facebook.com/groups`
	groupID  = `900072927547214`
	timeout  = 15
)

func main() {
	// TODO: Priority 4: Add a way to pass the following as command line arguments
	// Rate limit to limit the number of parses per second
	// Post size to limit the number of posts
	// Output folder to save the images to
	// Filenames for the csvs
	// Make it optional to scrape comments and attachments

	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	log.Println("Retrieving facebook group...")
	feed, err := getFacebookGroupFeed(ctx, groupID)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Retrieving posts...")
	var posts []*cdp.Node
	for i := 0; i < 5; i++ {
        if err := chromedp.Run(ctx,
			// Check if all content is loaded, at most 5 posts are loaded at a time
			// along with 3 nodes for the next set of posts breakpoint, loading indicator, and a buffer element
			chromedp.Poll(`document.querySelector('[data-pagelet="GroupFeed"] > [role="feed"]').children.length >= 8`,
				nil,
				chromedp.WithPollingInFrame(feed),
				chromedp.WithPollingMutation(),
				chromedp.WithPollingTimeout(timeout*time.Second),
			),
			// Get all the visible posts from the group feed
			chromedp.Nodes(`[aria-posinset][role="article"]`, &posts, chromedp.ByQueryAll, chromedp.FromNode(feed)),
		); err != nil {
			log.Fatal(err)
		}

		// TODO: Priority 2, start parsing data from the posts
		// Should generally be composed of four parts
		// 1. Parse the post content along with the author, date of post, and the content, as well as a generated ID or post ID if possible
		// 2. Attachment parsing if there are attachments, this links to the post ID, as well as the attachment link, and the type (image / video)
		// 3. Comments parsing if there are comments, this links to the post ID, as well as the comment ID, parent ID, the author, and the comment content
		// 4. Attachments should be downloaded to a folder with the post ID as the name

		// those above are required if I want to make this a generic scraper, but for now I just want to scrape snake data
		// thus the only thing I need are the images, the locations, and optionally the name of the snakes
		var content string
		for _, post := range posts {
			if err := chromedp.Run(ctx,
				chromedp.InnerHTML(`[data-ad-comet-preview="message"] span`, &content, chromedp.ByQuery, chromedp.FromNode(post)),
			); err != nil {
				log.Fatal(err)
			}

			log.Printf("Post content: %s", content)
		}

		if err := chromedp.Run(ctx,
			// Remove the posts that have already been parsed from the dom
			chromedp.Evaluate(`document.querySelectorAll('[data-pagelet="GroupFeed"] > [role="feed"] div:nth-last-child(n+4)').forEach((n) => n.remove());`, nil),
		); err != nil {
			log.Fatal(err)
		}
	}

	// TODO: Priority 3: Potential end of page reached, check what it looks like, and how to handle it
}

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
