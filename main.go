package main

import (
	"context"
	"log"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
)

const (
	endpoint = `https://www.facebook.com/groups/900072927547214/`
)

func main() {
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	log.Println("Closing login popup...")
	err := chromedp.Run(ctx,
		// Navigate to the group page
		chromedp.Navigate(endpoint),
		// Wait for the login popup to appear
		chromedp.WaitVisible(`#login_popup_cta_form`, chromedp.ByQuery),
		// Close the login popup
		chromedp.Click(`[role="dialog"] > div > [role="button"]`, chromedp.ByQuery, chromedp.NodeVisible),
		// Move the page to render it completely, this ensures that Facebook is able to 
        // append a component into the feed section which is used as a breakpoint to load more posts
		chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight);`, nil),
	)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Retrieving posts...")
	for i := 0; i < 5; i++ {
		var posts []*cdp.Node
		err = chromedp.Run(ctx,
			// Check if all posts have been loaded, for now we just wait 1 sec, there should be a better way to do this, maybe checking for a node that still is in shimmer
			// TODO: Maybe there is a better way to check whether new items have been loaded into the DOM
			chromedp.Sleep(3*time.Second),
			// Get all the visible posts from the group feed
			chromedp.Nodes(`[data-pagelet="GroupFeed"] > [role="feed"] [aria-posinset][role="article"]`, &posts, chromedp.ByQueryAll),
		)
		if err != nil {
			log.Fatal(err)
		}

		var content string
		for _, post := range posts {
			err = chromedp.Run(ctx,
				chromedp.InnerHTML(`[data-ad-comet-preview="message"] span`, &content, chromedp.ByQuery, chromedp.FromNode(post)),
			)
			if err != nil {
				log.Fatal(err)
			}

			log.Printf("Post content: %s", content)
		}

		err = chromedp.Run(ctx,
			// Remove the posts that have already been parsed from the dom
			chromedp.Evaluate(`document.querySelectorAll('[data-pagelet="GroupFeed"] > [role="feed"] > div:nth-last-child(n+4)').forEach((n) => n.remove());`, nil),
		)
		if err != nil {
			log.Fatal(err)
		}
	}
}
