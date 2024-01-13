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

	log.Printf("Navigating to group page (id: %s)...\n", groupID)
	if err := chromedp.Run(ctx, chromedp.Navigate(fmt.Sprintf("%s/%s/", endpoint, groupID))); err != nil {
		log.Fatal(err)
	}

	log.Println("Closing login popup...")
	if err := chromedp.Run(ctx,
		chromedp.WaitVisible(`#login_popup_cta_form`, chromedp.ByQuery),
		chromedp.Click(`[role="dialog"] > div > [role="button"]`, chromedp.ByQuery, chromedp.NodeVisible),
	); err != nil {
		log.Fatal(err)
	}

	log.Println("Retrieving feed...")
	var nodes []*cdp.Node
	if err := chromedp.Run(ctx, chromedp.Nodes(`[data-pagelet="GroupFeed"] > [role="feed"]`, &nodes, chromedp.ByQuery)); err != nil {
		log.Fatal(err)
	}
	if len(nodes) == 0 {
		log.Fatal("error: unable to extract feed")
	}
	feed := nodes[0]

	log.Println("Loading posts...")
	if err := chromedp.Run(ctx,
		// Removes a buffer element at the top of the feed, which is not really used
		chromedp.Evaluate(`document.querySelector('[data-pagelet="GroupFeed"] > [role="feed"] > div:first-child').remove();`, nil),
		// Move the page to render it completely, this ensures that Facebook is able to
		// append a component into the feed section which is used as a breakpoint to load more posts
		chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight);`, nil),
	); err != nil {
		log.Fatal(err)
	}

	log.Println("Extracting posts...")
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
