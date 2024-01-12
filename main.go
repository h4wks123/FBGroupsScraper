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
	scroll   = `
        const scrolls = 50;

        let scrollCount = 0;
        const scrollInterval = setInterval(() => {
            window.scrollTo(0, document.body.scrollHeight);
            scrollCount++;

            if (scrolls === numScrolls) {
                clearInterval(scrollInterval);
            }
        }, 500);
    `
)

func main() {
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	// Create a timeout
	// ctx, cancel = context.WithTimeout(ctx, 15*time.Second)
	// defer cancel()

	// navigate to a page, wait for an element, click
	log.Println("Retrieving posts...")
	var postNodes []*cdp.Node
	err := chromedp.Run(ctx,
		// Navigate to the FB page
		chromedp.Navigate(endpoint),
		// Wait for the login popup to appear, and close it
		chromedp.WaitVisible(`#login_popup_cta_form`, chromedp.ByQuery),
		chromedp.Click(`[role="dialog"] > div > [role="button"]`, chromedp.ByQuery, chromedp.NodeVisible),
		// Scroll the page
		chromedp.Evaluate(scroll, nil),
		chromedp.Sleep(25*time.Second),
		// Retrieve all the visible post nodes
		chromedp.Nodes(`[data-pagelet="GroupFeed"] > [role="feed"] [aria-posinset][role="article"]`, &postNodes, chromedp.ByQueryAll),
	)
	if err != nil {
		log.Fatal(err)
	}

	// TODO
	// keep track of the last posinset found in the group feed
	// scroll the page
	// parse the nodes again, etc etc making sure to only add new nodes to the list
	// update the posinset
	// repeat

	// parse the nodes collected to a struct
	// for this one we can optimize this later on to basically run alongside with the parsing so not alot of nodes
	// are stored in memory, and we don't need to maintain a list of nodes to parse

	log.Println("Found posts: ", len(postNodes))
	log.Println("Parsing content...")
	var content string
	for _, node := range postNodes {
		err = chromedp.Run(ctx,
			chromedp.InnerHTML(`[data-ad-comet-preview="message"] span > div`, &content, chromedp.ByQuery, chromedp.FromNode(node)),
		)
		if err != nil {
			log.Fatal(err)
		}

		log.Printf("Post content: %s", content)
	}
}
