package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/playwright-community/playwright-go"
	"github.com/yosssi/gohtml"
)

func main() {
	rawURL := "https://www.letsjive.io/"

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		log.Fatalf("could not parse URL: %v", err)
	}

	log.Printf("Downloading page: %s - to: %s", rawURL, parsedURL.Host)

	err = downloadPage(rawURL, parsedURL.Host)
	if err != nil {
		log.Fatalf("could not download page: %v", err)
	}
}

func downloadPage(url string, pageDir string) error {
	// Install Playwright
	err := playwright.Install()
	if err != nil {
		return fmt.Errorf("could not install Playwright: %v", err)
	}

	// Start Playwright
	log.Println("Starting Playwright...")
	pw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("could not start Playwright: %v", err)
	}
	defer pw.Stop()

	// Launch a browser
	log.Println("Launching browser...")
	browser, err := pw.Chromium.Launch()
	if err != nil {
		return fmt.Errorf("could not launch browser: %v", err)
	}
	defer browser.Close()

	// Create a new page
	page, err := browser.NewPage()
	if err != nil {
		return fmt.Errorf("could not create page: %v", err)
	}

	// Navigate to the page
	log.Printf("Navigating to %s...", url)
	_, err = page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	})
	if err != nil {
		return fmt.Errorf("could not navigate to page: %v", err)
	}

	// Create a directory for the page
	if err := os.MkdirAll(pageDir, 0755); err != nil {
		return fmt.Errorf("could not create directory %s: %v", pageDir, err)
	}

	// Take a screenshot of the full page
	err = takeScreenshot(filepath.Join(pageDir, "screenshot1.png"), page)
	if err != nil {
		return fmt.Errorf("could not take screenshot: %v", err)
	}

	// Delete junk elements
	junkElements := []string{
		"onetrust",
		"cookie",
	}
	for _, selector := range junkElements {
		// delete anything where the ID or class contains the selector
		_, err = page.Evaluate(fmt.Sprintf(`
			document.querySelectorAll('[id*="%s"]').forEach(e => e.remove());
			document.querySelectorAll('[class*="%s"]').forEach(e => e.remove());
		`, selector, selector))
		if err != nil {
			return fmt.Errorf("could not delete junk elements: %v", err)
		}
	}

	// Remove all JavaScript from the page
	log.Println("Removing JavaScript...")
	_, err = page.Evaluate(`document.querySelectorAll("script").forEach(s => s.remove())`)
	if err != nil {
		return fmt.Errorf("could not remove JavaScript: %v", err)
	}

	// Take a screenshot of the full page
	err = takeScreenshot(filepath.Join(pageDir, "screenshot2_nojs.png"), page)
	if err != nil {
		return fmt.Errorf("could not take screenshot: %v", err)
	}

	// Delete invisible elements
	log.Println("Deleting invisible elements...")
	_, err = page.Evaluate(`
		document.querySelectorAll("body *").forEach(e => {
			const style = getComputedStyle(e);
			if (style.display === "none" || style.visibility === "hidden") {
				e.remove();
			}
		});
	`)
	if err != nil {
		return fmt.Errorf("could not delete invisible elements: %v", err)
	}

	// Take a screenshot of the full page
	err = takeScreenshot(filepath.Join(pageDir, "screenshot3_no_invisible.png"), page)
	if err != nil {
		return fmt.Errorf("could not take screenshot: %v", err)
	}

	// Make sticky elements non-sticky
	log.Println("Making sticky elements non-sticky...")
	_, err = page.Evaluate(`
		document.querySelectorAll("*").forEach(e => {
			const style = getComputedStyle(e);
			if (style.position === "sticky") {
				e.style.position = "static";
			}
		});
	`)
	if err != nil {
		return fmt.Errorf("could not make sticky elements non-sticky: %v", err)
	}

	// Inline all CSS styles
	log.Println("Inlining CSS...")
	_, err = page.Evaluate(`
		(async function() {
			const styles = document.querySelectorAll("link[rel=stylesheet]");
			for (const link of styles) {
				const style = document.createElement("style");
				const result = await fetch(link.href);
				const text = await result.text();
				const tag = document.createElement("style");
				tag.textContent = text;
				link.replaceWith(tag);
			}
		})();
	`)
	if err != nil {
		return fmt.Errorf("could not inline CSS: %v", err)
	}

	// Wait until the script above has finished
	err = page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State: playwright.LoadStateNetworkidle,
	})
	if err != nil {
		return fmt.Errorf("could not wait for network idle: %v", err)
	}

	// Take a screenshot of the full page
	err = takeScreenshot(filepath.Join(pageDir, "screenshot4_inline_css.png"), page)
	if err != nil {
		return fmt.Errorf("could not take screenshot: %v", err)
	}

	//
	// Image
	//

	// Extract image sources
	log.Println("Extracting image sources...")
	imgSrcs, err := page.Evaluate(`Array.from(document.querySelectorAll('img')).map(img => img.src)`)
	if err != nil {
		return fmt.Errorf("could not extract image sources: %v", err)
	}

	// Convert result to a slice of strings
	Imgsrcs, ok := imgSrcs.([]interface{})
	if !ok {
		return fmt.Errorf("could not convert image sources to strings")
	}

	// Extract images from CSS background images
	log.Println("Extracting CSS background images...")
	cssSrcs, err := page.Evaluate(`
		Array.from(document.querySelectorAll("*")).map(e => {
			const style = getComputedStyle(e);
			const bg = style.backgroundImage;
			if (bg && bg !== "none") {
				return bg.replace(/^url\(["']?/, "").replace(/["']?\)$/, "");
			}
		}).filter(Boolean);
	`)
	if err != nil {
		return fmt.Errorf("could not extract CSS background images: %v", err)
	}

	// Convert result to a slice of strings
	cssSrcsSlice, ok := cssSrcs.([]interface{})
	if !ok {
		return fmt.Errorf("could not convert CSS background images to strings")
	}

	srcs := append(Imgsrcs, cssSrcsSlice...)

	// Download images
	log.Println("Downloading images...")
	imgReplacements := make(map[string]string)
	for i, src := range srcs {
		imgURL, ok := src.(string)
		if !ok {
			continue
		}

		if imgURL == "" {
			continue
		}
		if imgURL[:5] == "data:" {
			continue
		}

		baseName := filepath.Base(imgURL)
		imgName := fmt.Sprintf("image_%d_%s", i, baseName)

		imgReplacements[imgURL] = imgName

		// Get the image
		resp, err := http.Get(imgURL)
		if err != nil {
			log.Printf("could not get image %s: %v", imgURL, err)
			continue
		}
		defer resp.Body.Close()

		// Save the image locally
		imgPath := filepath.Join(pageDir, imgName)
		file, err := os.Create(imgPath)
		if err != nil {
			log.Printf("could not create image file %s: %v", imgPath, err)
			continue
		}
		defer file.Close()

		_, err = io.Copy(file, resp.Body)
		if err != nil {
			log.Printf("could not save image %s: %v", imgPath, err)
			continue
		}

		log.Printf("Saved image: %s", imgPath)
	}

	// Replace image sources in the HTML
	for imgURL, imgName := range imgReplacements {
		_, err = page.Evaluate(fmt.Sprintf(`
			document.querySelectorAll('img[src="%s"]').forEach(img => {
				img.src = "%s";
			});
		`, imgURL, imgName))
		if err != nil {
			return fmt.Errorf("could not replace image source %s: %v", imgURL, err)
		}
	}

	// Wait until the script above has finished
	err = page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State: playwright.LoadStateNetworkidle,
	})
	if err != nil {
		return fmt.Errorf("could not wait for network idle: %v", err)
	}

	// Take a screenshot of the full page
	err = takeScreenshot(filepath.Join(pageDir, "screenshot5_image_replace.png"), page)
	if err != nil {
		return fmt.Errorf("could not take screenshot: %v", err)
	}

	//
	// CSS Cleanup
	//

	// Delete unused CSS rules
	_, err = page.Evaluate(`
		(function() {
			const usedStyles = new Set();
			[...document.styleSheets].forEach(sheet => {
				try {
					for (let i = 0; i < sheet.cssRules.length; i++) {
						const rule = sheet.cssRules[i];
						const selector = rule.selectorText;
						if (rule.conditionText) {
							if (window.matchMedia(rule.conditionText).matches) {
								for (let j = 0; j < rule.cssRules.length; j++) {
									const subRule = rule.cssRules[j];
									const subSelector = subRule.selectorText;

									if (!!document.querySelector(subSelector)) {
										usedStyles.add(subRule);
									}
								}
							}
						} else {
							if (!!document.querySelector(selector)) {
								usedStyles.add(rule);
							}
						}
					}
				} catch (e) {
					console.warn("Error processing stylesheet:", e);
				}
			});

			// Add a style tag with all the used styles
			const style = document.createElement("style");
			style.id = "used-styles";
			style.textContent = [...usedStyles].map(rule => rule.cssText).join("\n");
			document.head.appendChild(style);

			// Delete all other style tags
			document.querySelectorAll("style").forEach(s => {
				if (s.id !== "used-styles") {
					s.remove();
				}
			});
		})();
	`)
	if err != nil {
		return fmt.Errorf("could not delete unused CSS rules: %v", err)
	}

	// Wait until the script above has finished
	err = page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State: playwright.LoadStateNetworkidle,
	})
	if err != nil {
		return fmt.Errorf("could not wait for network idle: %v", err)
	}

	// Delete meta tags except http-equiv="Content-Type"
	log.Println("Deleting meta tags...")
	_, err = page.Evaluate(`
		document.querySelectorAll("meta").forEach(meta => {
			if (meta.getAttribute("http-equiv") !== "Content-Type") {
				meta.remove();
			}
		});

		// If there's no meta charset tag, add one
		if (!document.querySelector('meta[charset]')) {
			const meta = document.createElement("meta");
			meta.setAttribute("charset", "UTF-8");
			document.head.appendChild(meta);
		}
	`)
	if err != nil {
		return fmt.Errorf("could not delete meta tags: %v", err)
	}

	// Delete link tags
	log.Println("Deleting link tags...")
	_, err = page.Evaluate(`
		document.querySelectorAll("link").forEach(link => link.remove());
	`)
	if err != nil {
		return fmt.Errorf("could not delete link tags: %v", err)
	}

	// Delete HTML comments
	log.Println("Deleting HTML comments...")
	_, err = page.Evaluate(`
		const iterator = document.createNodeIterator(
			document,
			NodeFilter.SHOW_COMMENT
		);
		let currentNode;
		while ((currentNode = iterator.nextNode())) {
			currentNode.parentNode.removeChild(currentNode);
		}
	`)
	if err != nil {
		return fmt.Errorf("could not delete HTML comments: %v", err)
	}

	// Take a screenshot of the full page
	log.Println("Taking screenshot after...")
	bytes2, err := page.Screenshot(playwright.PageScreenshotOptions{
		FullPage: playwright.Bool(true),
	})
	if err != nil {
		return fmt.Errorf("could not take screenshot: %v", err)
	}

	// Save the screenshot to a file
	log.Println("Saving screenshot after...")
	if err = os.WriteFile("screenshot2.png", bytes2, 0644); err != nil {
		return fmt.Errorf("could not save screenshot: %v", err)
	}

	// Get the full rendered HTML
	html, err := page.Content()
	if err != nil {
		return fmt.Errorf("could not get HTML: %v", err)
	}

	// Format the HTML so it looks nice
	newHtml := gohtml.Format(html)

	// Save the HTML to a file
	if err = os.WriteFile(filepath.Join(pageDir, "index.html"), []byte(newHtml), 0644); err != nil {
		return fmt.Errorf("could not save HTML: %v", err)
	}

	return nil
}

func takeScreenshot(name string, page playwright.Page) error {
	// Take a screenshot of the full page
	log.Println("Taking screenshot...")
	bytes, err := page.Screenshot(playwright.PageScreenshotOptions{
		FullPage: playwright.Bool(true),
	})
	if err != nil {
		return fmt.Errorf("could not take screenshot: %v", err)
	}

	// Save the screenshot to a file
	log.Println("Saving screenshot...")
	if err = os.WriteFile(name, bytes, 0644); err != nil {
		return fmt.Errorf("could not save screenshot: %v", err)
	}

	return nil
}
