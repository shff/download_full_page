package main

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/HugoSmits86/nativewebp"
	"github.com/mxschmitt/playwright-go"
	"github.com/yosssi/gohtml"
)

var inlineThreshold = 2048
var debug = true

func main() {
	if len(os.Args) < 2 || os.Args[1] == "" {
		log.Fatalf("usage: %s <url>", os.Args[0])
	}
	rawURL := os.Args[1]

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		log.Fatalf("could not parse URL: %v", err)
	}

	log.Printf("🛜 Processing page: %s - to: %s", rawURL, parsedURL.Host)
	err = downloadPage(rawURL, parsedURL.Host)
	if err != nil {
		log.Fatalf("could not download page: %v", err)
	}
}

func downloadPage(url string, pageDir string) error {
	debugDir := pageDir + "_debug"

	headless := true

	deleteJunk := true
	deleteIframes := true
	removeJavascript := true
	deleteInvisibleElements := true
	removeOverlays := true
	makeElementsNonSticky := false
	inlineCSS := true
	cssRuleCleanup := true
	extractImages := true
	deleteMeta := true
	deleteLink := true
	deleteAttributes := true
	deleteEmptyStyles := true
	deleteAria := true
	deleteComments := true

	// Install Playwright
	err := playwright.Install()
	if err != nil {
		return fmt.Errorf("could not install Playwright: %v", err)
	}

	// Start Playwright
	log.Println("🎬 Starting Playwright...")
	pw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("could not start Playwright: %v", err)
	}
	defer pw.Stop()

	// Launch a browser
	log.Println("🚀 Launching browser...")
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(headless),
		Args: []string{
			"--disable-web-security",
			"--disable-features=IsolateOrigins,site-per-process",
		},
	})
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
	log.Printf("🧭 Navigating to %s...", url)
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

	if debug {
		// Create a directory for the page
		if err := os.MkdirAll(debugDir, 0755); err != nil {
			return fmt.Errorf("could not create directory %s: %v", pageDir, err)
		}

		// Take a screenshot of the full page
		err = takeScreenshot(filepath.Join(debugDir, "screenshot1.png"), page)
		if err != nil {
			return fmt.Errorf("could not take screenshot: %v", err)
		}
	}

	//
	// Delete Junk
	//

	if deleteJunk {
		// Delete junk elements
		junkElements := []string{
			"onetrust",
			"cookie",
			"Cookie",
			"intercom",
			"ch2-region-c1",
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
	}

	//
	// Delete Iframes
	//

	if deleteIframes {
		// Delete iframes
		log.Println("🖼️ Deleting iframes...")
		_, err = page.Evaluate(`
			document.querySelectorAll("iframe").forEach(iframe => iframe.remove());
		`)
		if err != nil {
			return fmt.Errorf("could not delete iframes: %v", err)
		}
	}

	// Scroll down to the bottom very slowly, then to the top again
	log.Println("🚶‍♂️ Scrolling down...")
	_, err = page.Evaluate(`
		(async function() {
			for (let i = 0; i < 10; i++) {
				window.scrollBy(0, window.innerHeight);
				await new Promise(resolve => setTimeout(resolve, 100));
			}
			window.scrollBy(0, 0);
		})();
	`)
	if err != nil {
		return fmt.Errorf("could not scroll down: %v", err)
	}

	// Wait until the script above has finished
	err = page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State: playwright.LoadStateNetworkidle,
	})
	if err != nil {
		return fmt.Errorf("could not wait for network idle: %v", err)
	}

	// Scroll back to the top
	_, err = page.Evaluate(`window.scrollTo(0, 0)`)
	if err != nil {
		return fmt.Errorf("could not scroll to top: %v", err)
	}

	//
	// Remove Javascript
	//

	if removeJavascript {
		// Stop javascript from running
		log.Println("🛑 Stopping JavaScript...")
		_, err = page.Evaluate(`
			window.stop();

			// Stop all timers
			for (const id of setTimeout(() => {}).toString().match(/\d+/g)) {
				clearTimeout(id);
			}
			for (const id of setInterval(() => {}).toString().match(/\d+/g)) {
				clearInterval(id);
			}
		`)
		if err != nil {
			return fmt.Errorf("could not stop page: %v", err)
		}

		// Remove all JavaScript from the page
		log.Println("🧹 Removing JavaScript...")
		_, err = page.Evaluate(`document.querySelectorAll("script").forEach(s => s.remove())`)
		if err != nil {
			return fmt.Errorf("could not remove JavaScript: %v", err)
		}

		// Wait until the script above has finished
		err = page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
			State:   playwright.LoadStateNetworkidle,
			Timeout: playwright.Float(2000),
		})
		if err != nil {
			log.Printf("could not wait for network idle: %v\n", err)
		}

		if debug {
			// Take a screenshot of the full page
			err = takeScreenshot(filepath.Join(debugDir, "screenshot2a_nojs.png"), page)
			if err != nil {
				return fmt.Errorf("could not take screenshot: %v", err)
			}
		}
	}

	//
	// Restart from scratch
	//

	if true {
		_, err := page.Evaluate(`
			const head = document.head.outerHTML;
			document.head.innerHTML = "";
			document.head.innerHTML = head;

			const html = document.documentElement.outerHTML;
			document.documentElement.innerHTML = "";
			document.documentElement.innerHTML = html;
		`)
		if err != nil {
			return fmt.Errorf("could not restart from scratch: %v", err)
		}

		// Wait until the script above has finished
		err = page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
			State: playwright.LoadStateNetworkidle,
		})
		if err != nil {
			return fmt.Errorf("could not wait for network idle: %v", err)
		}

		if debug {
			// Take a screenshot of the full page
			err = takeScreenshot(filepath.Join(debugDir, "screenshot2b_restart_from_scratch.png"), page)
			if err != nil {
				return fmt.Errorf("could not take screenshot: %v", err)
			}
		}
	}

	//
	// Delete Invisible Elements
	//

	if deleteInvisibleElements {
		// Delete invisible elements
		log.Println("👻 Deleting invisible elements...")
		_, err = page.Evaluate(`
			document.querySelectorAll("body *:not(link)").forEach(e => {
				const style = getComputedStyle(e);

				// if it's an SVG that contains symbols linked elsewhere, don't remove it
				if (e.tagName === "svg" && e.querySelector("symbol")) {
					return;
				}
				if (style.display === "none" || style.visibility === "hidden") {
					e.remove();
				}
			});
		`)
		if err != nil {
			return fmt.Errorf("could not delete invisible elements: %v", err)
		}

		if debug {
			// Take a screenshot of the full page
			err = takeScreenshot(filepath.Join(debugDir, "screenshot3_no_invisible.png"), page)
			if err != nil {
				return fmt.Errorf("could not take screenshot: %v", err)
			}
		}
	}

	//
	// Remove Overlays (cookie banners, consent walls, chat widgets, sticky CTAs)
	//

	if removeOverlays {
		log.Println("🍪 Removing overlays...")
		result, err := page.Evaluate(`
			(function() {
				const vw = window.innerWidth;
				const vh = window.innerHeight;

				// Consent / widget vocabulary, kept specific to avoid matching an
				// article that merely discusses cookies.
				const keywords = [
					"accept cookies", "accept all", "reject all", "cookie policy",
					"cookie settings", "cookie preferences", "manage cookies",
					"we value your privacy", "we use cookies", "this site uses cookies",
					"privacy preferences", "manage preferences", "consent",
					"gdpr", "ccpa", "your privacy choices",
				];
				const keywordRe = new RegExp(keywords.map(k =>
					k.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")
				).join("|"), "i");

				let removed = 0;
				document.querySelectorAll("body *").forEach(e => {
					if (!e.isConnected) return; // parent already removed
					const style = getComputedStyle(e);
					if (style.position !== "fixed" && style.position !== "sticky") {
						return;
					}

					const rect = e.getBoundingClientRect();
					if (rect.width === 0 || rect.height === 0) return;

					// Don't remove the real content: something covering most of the
					// page AND holding most of its text.
					const coversPage = rect.width * rect.height > vw * vh * 0.6;
					const bodyTextLen = document.body.innerText.length || 1;
					const holdsContent = (e.innerText || "").length > bodyTextLen * 0.5;
					if (coversPage && holdsContent) return;

					const z = parseInt(style.zIndex, 10) || 0;
					const edge =
						rect.bottom >= vh - 4 ||
						rect.top <= 4 ||
						rect.right >= vw - 4 ||
						rect.left <= 4;

					const looksLikeOverlay = edge && z >= 1000;
					const hasConsentText = keywordRe.test(e.innerText || "") ||
						keywordRe.test(e.className || "") ||
						keywordRe.test(e.id || "");

					if (looksLikeOverlay || hasConsentText) {
						e.remove();
						removed++;
					}
				});

				return removed;
			})();
		`)
		if err != nil {
			return fmt.Errorf("could not remove overlays: %v", err)
		}
		log.Printf("   removed %v overlay element(s)", result)

		if debug {
			err = takeScreenshot(filepath.Join(debugDir, "screenshot3a_no_overlays.png"), page)
			if err != nil {
				return fmt.Errorf("could not take screenshot: %v", err)
			}
		}
	}

	//
	// Make Elements Non-Sticky
	//

	if makeElementsNonSticky {
		// Make sticky elements non-sticky
		log.Println("🏒 Making sticky elements non-sticky...")
		_, err = page.Evaluate(`
		document.querySelectorAll("*").forEach(e => {
			const style = getComputedStyle(e);
			if (style.position === "sticky" || style.position === "fixed") {
				e.style.position = "static";
			}
		});
	`)
		if err != nil {
			return fmt.Errorf("could not make sticky elements non-sticky: %v", err)
		}

		if debug {
			// Take a screenshot of the full page
			err = takeScreenshot(filepath.Join(debugDir, "screenshot3b_remove_sticky.png"), page)
			if err != nil {
				return fmt.Errorf("could not take screenshot: %v", err)
			}
		}
	}

	//
	// Inline CSS
	//

	if inlineCSS {
		// Inline all CSS styles
		log.Println("💅 Inlining CSS...")
		_, err = page.Evaluate(`
		(function() {
			// Resolve relative url(...) against the stylesheet's own URL.
			function absolutizeUrls(cssText, baseHref) {
				return cssText.replace(/url\(\s*(['"]?)([^'")]+)\1\s*\)/g, (m, q, u) => {
					if (/^(data:|https?:|#)/i.test(u)) return m;
					try { return 'url("' + new URL(u, baseHref).href + '")'; }
					catch (e) { return m; }
				});
			}

			const links = [...document.querySelectorAll("link[rel=stylesheet]")];

			// Fetch all stylesheets first, preserving order and the cascade.
			return Promise.all(links.map(link =>
				fetch(link.href)
					.then(r => r.ok ? r.text() : null)
					.then(text => ({ link, media: link.media || "", text: text == null ? null : absolutizeUrls(text, link.href) }))
					.catch(() => ({ link, media: link.media || "", text: null }))
			)).then(results => {
				for (const { link, media, text } of results) {
					// Print-only sheets are inactive on screen but would hide
					// screen chrome (nav/header) if inlined unconditionally.
					if (media.trim().toLowerCase() === "print") { link.remove(); continue; }
					if (text == null) continue; // fetch failed: keep the <link>
					const tag = document.createElement("style");
					tag.textContent = (media && media !== "all")
						? "@media " + media + " {\n" + text + "\n}"
						: text;
					link.replaceWith(tag);
				}
			});
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

		if debug {
			// Take a screenshot of the full page
			err = takeScreenshot(filepath.Join(debugDir, "screenshot4_inline_css.png"), page)
			if err != nil {
				return fmt.Errorf("could not take screenshot: %v", err)
			}
		}
	}

	//
	// CSS Cleanup
	//

	if cssRuleCleanup {
		// Delete unused CSS rules
		_, err = page.Evaluate(`
			(function() {
				const usedStyles = new Set();
				const unusedStyles = new Set();

				[...document.styleSheets].forEach(sheet => {
					for (let i = 0; i < sheet.cssRules.length; i++) {
						const rule = sheet.cssRules[i];
						let selector = rule.selectorText;

						// Delete white-space pre/prewrap
						if (rule.style?.whiteSpace === "pre-wrap" || rule.style?.whiteSpace === "pre") {
							rule.style.removeProperty("white-space");
						}

						if (!rule.conditionText) {
							const selectorText = selector?.replace(/::[a-z]+/g, '');
							try {
								if (!!document.querySelector(selectorText)) {
									usedStyles.add(rule);
								}
							} catch (e) {
								usedStyles.add(rule);
							}
						} else {
							if (window.matchMedia(rule.conditionText).matches) {
								for (let j = 0; j < rule.cssRules.length; j++) {
									const subRule = rule.cssRules[j];
									const subSelector = subRule.selectorText;
									const subSelectorText = subSelector?.replace(/::?[a-z]+/g, '');

									// Delete white-space pre/prewrap
									if (subRule.style?.whiteSpace === "pre-wrap" || subRule.style?.whiteSpace === "pre") {
										subRule.style.removeProperty("white-space");
									}

									try {
										if (true || !!document.querySelector(subSelectorText)) {
											// const mediaRule = new CSSMediaRule();
											// mediaRule.conditionText = rule.conditionText;
											// mediaRule.insertRule(subRule.cssText);
											// usedStyles.add(mediaRule);
											usedStyles.add(subRule);
										}
									} catch (e) {
										usedStyles.add(subRule);
									}
								}
							}
						}
					}
				});

				// Add a style tag with all the used styles
				const style = document.createElement("style");
				style.id = "used-styles";
				style.textContent = [...usedStyles].map(rule => rule.cssText).join("\n");
				document.head.appendChild(style);

				// Delete all other style tags
				document.querySelectorAll("style").forEach(s => {
					if (s.id !== "used-styles" && s.id !== "unused-styles") {
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

		if debug {
			// Take a screenshot of the full page
			err = takeScreenshot(filepath.Join(debugDir, "screenshot6_after_css_cleanup.png"), page)
			if err != nil {
				return fmt.Errorf("could not take screenshot: %v", err)
			}
		}
	}

	//
	// Image
	//

	if extractImages {
		// Extract image sources
		log.Println("Extracting image sources...")
		imgImages, err := page.Evaluate(`Array.from(document.querySelectorAll('img')).map(img => img.src)`)
		if err != nil {
			return fmt.Errorf("could not extract image sources: %v", err)
		}

		// Convert result to a slice of strings
		imgImagesSlice, ok := imgImages.([]interface{})
		if !ok {
			return fmt.Errorf("could not convert image sources to strings")
		}

		// Extract images from CSS background images
		log.Println("Extracting CSS background images...")
		cssImages, err := page.Evaluate(`
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
		cssImagesSlice, ok := cssImages.([]interface{})
		if !ok {
			return fmt.Errorf("could not convert CSS background images to strings")
		}

		images := append(imgImagesSlice, cssImagesSlice...)

		// Download images
		log.Println("Downloading images...")
		imgReplacements := make(map[string]string)
		for _, src := range images {
			imgURL, ok := src.(string)
			if !ok {
				continue
			}

			if imgURL == "" {
				continue
			}
			if strings.HasPrefix(imgURL, "data:") {
				continue
			}

			// Get the image
			resp, err := http.Get(imgURL)
			if err != nil {
				// log.Printf("could not get image %s: %v", imgURL, err)
				continue
			}

			// Get the image into a byte slice
			imgBytes, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				log.Printf("could not read image %s: %v", imgURL, err)
				continue
			}

			// Get the md5 hash of the downloaded file
			hash := md5.Sum(imgBytes)
			hashString := hex.EncodeToString(hash[:])

			baseName := filepath.Base(imgURL)
			extension := filepath.Ext(baseName)
			imgName := fmt.Sprintf("image_%s%s", hashString, extension)

			imgReplacements[imgURL] = imgName

			// Check if the file doesn't exist in the directory yet
			if _, err := os.Stat(filepath.Join(pageDir, imgName)); err == nil {
				// log.Printf("Image %s already exists, skipping", imgName)
				continue
			}

			// Convert PNG images to WebP
			if extension == ".png" {
				// Decode the input image bytes
				img, err := png.Decode(bytes.NewReader(imgBytes))
				if err != nil {
					// originalPath := filepath.Join(pageDir, imgName)
					// log.Printf("could not decode image %s: %v", originalPath, err)
					continue
				}

				// Create a buffer to hold the WebP bytes
				var imageOutput bytes.Buffer

				// Encode the image into WebP format (lossless VP8L)
				if err := nativewebp.Encode(&imageOutput, img, &nativewebp.Options{
					CompressionLevel: nativewebp.DefaultCompression,
				}); err != nil {
					log.Printf("could not encode image %s to WebP: %v", imgName, err)
					continue
				}

				// Use a new name for the WebP image
				imgName = fmt.Sprintf("image_%s.webp", hashString)
				imgPath := filepath.Join(pageDir, imgName)

				// Check if file is smaller than 2kb
				if len(imageOutput.Bytes()) < inlineThreshold {
					// Create a data: URL for the WebP image
					webpDataURL := fmt.Sprintf("data:image/webp;base64,%s", base64.StdEncoding.EncodeToString(imageOutput.Bytes()))

					// Replace the image source in the HTML
					imgReplacements[imgURL] = webpDataURL

					log.Printf("Converted image %s to WebP -- INLINED", imgPath)
				} else {
					// Save the image locally
					if err := os.WriteFile(imgPath, imageOutput.Bytes(), 0644); err != nil {
						log.Printf("could not write image file %s: %v", imgPath, err)
						continue
					}

					// We should replace the image source in the HTML
					imgReplacements[imgURL] = imgName

					// log.Printf("Saved image: %s from %s", imgPath, imgURL)
				}
			} else if extension == ".svg" {
				imgPath := filepath.Join(pageDir, imgName)

				// Save the image locally
				if err := os.WriteFile(imgPath, imgBytes, 0644); err != nil {
					log.Printf("could not write image file %s: %v", imgPath, err)
					continue
				}

				originalSize := len(imgBytes)

				// Optimize the SVG image
				cmd := exec.Command("svgo", "-i", imgPath, "-o", imgPath, "-p", "3")
				err = cmd.Run()
				if err != nil {
					log.Printf("could not optimize SVG image %s: %v", imgPath, err)
				} else {
					optimizedBytes, err := os.ReadFile(imgPath)
					if err != nil {
						log.Printf("could not read optimized SVG image %s: %v", imgPath, err)
					} else {
						optimizedSize := len(optimizedBytes)
						log.Printf("Optimized SVG image %s: %d -> %d bytes", imgPath, originalSize, optimizedSize)
					}
				}

				log.Printf("Saved SVG image: %s from %s", imgPath, imgURL)

				// Read the optimized SVG image
				imgBytes, err = os.ReadFile(imgPath)
				if err != nil {
					log.Printf("could not read optimized SVG image %s: %v", imgPath, err)
					continue
				}

				// Check if the image has less than 2kb
				if len(imgBytes) < inlineThreshold {
					// Create a data: URL for the SVG image
					svgDataURL := fmt.Sprintf("data:image/svg+xml;base64,%s", base64.StdEncoding.EncodeToString(imgBytes))

					// Replace the image source in the HTML
					imgReplacements[imgURL] = svgDataURL

					// Delete the local file
					err = os.Remove(imgPath)
					if err != nil {
						log.Printf("could not delete image file %s: %v", imgPath, err)
					}

					log.Printf("Optimized SVG image %s -- INLINED", imgPath)
				}
			} else {
				imgPath := filepath.Join(pageDir, imgName)

				// Save the image locally
				if err := os.WriteFile(imgPath, imgBytes, 0644); err != nil {
					log.Printf("could not write image file %s: %v", imgPath, err)
					continue
				}

				// Replace the image source in the HTML
				imgReplacements[imgURL] = imgName
			}
		}

		// Replace image sources in the HTML. Match on the browser-resolved
		// absolute img.src (the map is keyed by absolute URLs), not the raw
		// attribute, so relatively-referenced images get rewritten too.
		replacementsJSON, err := json.Marshal(imgReplacements)
		if err != nil {
			return fmt.Errorf("could not marshal image replacements: %v", err)
		}
		_, err = page.Evaluate(fmt.Sprintf(`
			const replacements = %s;
			document.querySelectorAll("img").forEach(img => {
				const local = replacements[img.src];
				if (local) {
					img.setAttribute("src", local);
				}
			});
		`, string(replacementsJSON)))
		if err != nil {
			return fmt.Errorf("could not replace image sources: %v", err)
		}

		// Set the base URL so bare local filenames resolve next to index.html
		_, err = page.Evaluate(`
			const base = document.createElement("base");
			base.href = "./";
			document.head.appendChild(base);
		`)
		if err != nil {
			return fmt.Errorf("could not change base URL: %v", err)
		}

		// Wait until the script above has finished
		err = page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
			State: playwright.LoadStateNetworkidle,
		})
		if err != nil {
			return fmt.Errorf("could not wait for network idle: %v", err)
		}

		if debug {
			// Take a screenshot of the full page
			err = takeScreenshot(filepath.Join(debugDir, "screenshot6_image_replace.png"), page)
			if err != nil {
				return fmt.Errorf("could not take screenshot: %v", err)
			}
		}
	}

	//
	// Extra Cleanup
	//

	if deleteMeta {
		// Delete meta tags except http-equiv="Content-Type"
		log.Println("💽 Deleting meta tags...")
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

		if debug {
			// Take a screenshot of the full page
			err = takeScreenshot(filepath.Join(debugDir, "screenshot7a_without_meta.png"), page)
			if err != nil {
				return fmt.Errorf("could not take screenshot: %v", err)
			}
		}
	}

	if deleteLink {
		// Delete link tags
		log.Println("⛓️‍💥 Deleting link tags...")
		_, err = page.Evaluate(`
		document.querySelectorAll("link").forEach(link => link.remove());
	`)
		if err != nil {
			return fmt.Errorf("could not delete link tags: %v", err)
		}

		if debug {
			// Take a screenshot of the full page
			err = takeScreenshot(filepath.Join(debugDir, "screenshot7b_without_link.png"), page)
			if err != nil {
				return fmt.Errorf("could not take screenshot: %v", err)
			}
		}
	}

	if deleteAttributes {
		// Delete attributes starting with data-
		log.Println("✍🏻 Deleting attributes...")
		_, err = page.Evaluate(`
		document.querySelectorAll("*").forEach(e => {
			for (const attr of e.attributes) {
				if (attr.name.startsWith("data-")) {
					e.removeAttribute(attr.name);
				}
			}
		});
	`)
		if err != nil {
			return fmt.Errorf("could not delete data- attributes: %v", err)
		}

		if debug {
			// Take a screenshot of the full page
			err = takeScreenshot(filepath.Join(debugDir, "screenshot7c_without_data.png"), page)
			if err != nil {
				return fmt.Errorf("could not take screenshot: %v", err)
			}
		}
	}

	if deleteEmptyStyles {
		// Delete all empty class and style tags
		log.Println("🫥 Deleting empty class and style tags...")
		_, err = page.Evaluate(`
		document.querySelectorAll("*").forEach(e => {
			if (e.className === "") {
				e.removeAttribute("class");
			}
			if (e.style.cssText === "") {
				e.removeAttribute("style");
			}
		});
	`)
		if err != nil {
			return fmt.Errorf("could not delete empty class and style tags: %v", err)
		}

		if debug {
			// Take a screenshot of the full page
			err = takeScreenshot(filepath.Join(debugDir, "screenshot7d_without_empty_class.png"), page)
			if err != nil {
				return fmt.Errorf("could not take screenshot: %v", err)
			}
		}
	}

	if deleteAria {
		// Delete title and alt attributes
		log.Println("♿️ Deleting title, alt and aria attributes...")
		_, err = page.Evaluate(`
		document.querySelectorAll("*").forEach(e => {
			const attrs = e.getAttributeNames();
			for (const attr of attrs) {
				if (attr.startsWith("aria-")) {
					e.removeAttribute(attr);
				}
			}
			e.removeAttribute("title");
			e.removeAttribute("alt");
			e.removeAttribute("role");
			e.removeAttribute("itemscope");
			e.removeAttribute("itemtype");
		});
	`)
		if err != nil {
			return fmt.Errorf("could not delete title, alt and aria attributes: %v", err)
		}

		if debug {
			// Take a screenshot of the full page
			err = takeScreenshot(filepath.Join(debugDir, "screenshot7e_without_aria.png"), page)
			if err != nil {
				return fmt.Errorf("could not take screenshot: %v", err)
			}
		}
	}

	if deleteComments {
		// Delete HTML comments
		log.Println("💬 Deleting HTML comments...")
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

		if debug {
			// Take a screenshot of the full page
			err = takeScreenshot(filepath.Join(debugDir, "screenshot7f_without_comments.png"), page)
			if err != nil {
				return fmt.Errorf("could not take screenshot: %v", err)
			}
		}
	}

	if debug {
		// Take a screenshot of the full page
		err = takeScreenshot(filepath.Join(debugDir, "screenshot8_final_original.png"), page)
		if err != nil {
			return fmt.Errorf("could not take screenshot: %v", err)
		}
	}

	// Get the full rendered HTML
	html, err := page.Evaluate(`document.documentElement.outerHTML`)
	if err != nil {
		return fmt.Errorf("could not get HTML: %v", err)
	}

	// Format the HTML so it looks nice
	newHtml := gohtml.Format(html.(string))
	// newHtml = html.(string)

	// Save the HTML to a file
	log.Println("💾 Saving HTML...")
	if err = os.WriteFile(filepath.Join(pageDir, "index.html"), []byte(newHtml), 0644); err != nil {
		return fmt.Errorf("could not save HTML: %v", err)
	}

	if debug {
		absolutePath, err := filepath.Abs(filepath.Join(pageDir, "index.html"))
		if err != nil {
			return fmt.Errorf("could not resolve absolute path: %v", err)
		}

		// Open the page in the browser
		log.Println("🌐 Opening page in playwright...")
		_, err = page.Goto("file://" + absolutePath)
		if err != nil {
			return fmt.Errorf("could not open page in playwright: %v", err)
		}

		// Wait for the page to load
		err = page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
			State:   playwright.LoadStateNetworkidle,
			Timeout: playwright.Float(2000),
		})
		if err != nil {
			return fmt.Errorf("could not wait for network idle: %v", err)
		}

		// Take a screenshot of the full page
		err = takeScreenshot(filepath.Join(debugDir, "screenshot9_final_page.png"), page)
		if err != nil {
			return fmt.Errorf("could not take screenshot: %v", err)
		}
	}

	return nil
}

func takeScreenshot(name string, page playwright.Page) error {
	// Take a screenshot of the full page
	bytes, err := page.Screenshot(playwright.PageScreenshotOptions{
		FullPage: playwright.Bool(true),
		Timeout:  playwright.Float(2000),
	})
	if err != nil {
		return fmt.Errorf("could not take screenshot: %v", err)
	}

	// Save the screenshot to a file
	if err = os.WriteFile(name, bytes, 0644); err != nil {
		return fmt.Errorf("could not save screenshot: %v", err)
	}

	return nil
}
