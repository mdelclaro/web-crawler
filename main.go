package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/net/html"
)

var (
	target, dir string
	URLs        = []string{}
	mutex       = &sync.RWMutex{}
	wg          sync.WaitGroup
)

func main() {
	flag.StringVar(&target, "url", "", "target URL")
	flag.StringVar(&dir, "dir", "", "directory where files will be saved")
	flag.Parse()

	if target == "" {
		log.Fatal("url flag is required")
	}

	if !strings.HasPrefix(target, "http") {
		log.Fatal("invalid url provided. valid ex.: https://github.com")
	}

	if dir == "" {
		dir = "./data"
		println("dir flag is empty. using default ./data")
	}

	// listen to kill commands
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGINT)
	go func() {
		<-c
		println("\nstopping...")
		os.Exit(1)
	}()

	err := process(target)
	if err != nil {
		panic(err)
	}

	wg.Wait()

	println("done!")
}

func process(target string) error {
	// remove "/" suffix to avoid duplicating it
	target = strings.TrimSuffix(target, "/")
	parsedURL, err := url.Parse(target)
	if err != nil {
		fmt.Printf("error parsing the target: %v", err)
	}

	// parsing the target
	target = fmt.Sprintf("%v://%v%v", parsedURL.Scheme, parsedURL.Host, parsedURL.Path)

	ok := false

	for _, u := range URLs {
		if target == u {
			ok = true
		}
	}

	if !ok {
		mutex.Lock()
		URLs = append(URLs, target)
		mutex.Unlock()

		var content []byte
		fp := filepath.Join(dir, parsedURL.Path)
		fileName := path.Base(parsedURL.Path)

		// call it index in case it's the target
		if fileName == "." {
			fileName = "index"
		}

		// check for file existence
		savedContent := checkForFile(fp, fileName+".html")
		if savedContent == nil {
			// download page
			content, err = download(target)
			if err != nil {
				fmt.Printf("error downloading the target: %v", err)
			}

			// save page
			if err := save(fp, fileName+".html", content); err != nil {
				fmt.Printf("error saving the target: %v", err)
			}
		} else {
			content = savedContent
		}

		// parse page content
		htmlContent, err := parseHTML(content)
		if err != nil {
			fmt.Printf("error parsing html content: %v", err)
		}

		// extract urls from page
		urls, err := extractUrls(htmlContent, parsedURL)
		if err != nil {
			fmt.Printf("error extracting urls: %v", err)
		}

		// call process() for each found url recursively
		for _, u := range urls {
			wg.Add(1)

			go func(targetUrl string) {
				defer wg.Done()
				process(targetUrl)
			}(u)
		}
	}

	return nil
}

func download(url string) ([]byte, error) {
	println("downloading", url)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("invalid status code")
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func checkForFile(filePath string, fileName string) []byte {
	data, err := os.ReadFile(filePath + "/" + fileName)
	if err != nil {
		println(filePath, "does not exist. downloading and saving...")
		return nil
	}

	println(filePath, "already exists")

	return data
}

func save(filePath string, fileName string, data []byte) error {
	if err := os.MkdirAll(filePath, os.ModePerm); err != nil {
		return err
	}

	file, err := os.Create(filePath + "/" + fileName)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(data)
	if err != nil {
		return err
	}

	return nil
}

func parseHTML(data []byte) (*html.Node, error) {
	htmlDoc, err := html.Parse(strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}

	return htmlDoc, nil
}

func extractUrls(htlmDoc *html.Node, parsedURL *url.URL) ([]string, error) {
	println("extracting urls from ", parsedURL.Host+parsedURL.Path)

	invalidValues := []string{"#", "/"}
	urls := []string{}

	targetScheme := parsedURL.Scheme
	targetURL := parsedURL.Host + parsedURL.Path
	domain := parsedURL.Host

	// recursively search for <a> tags on html page
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key == "href" {
					newUrl := a.Val

					// check for invalid url values
					if strings.HasPrefix(newUrl, "#") {
						continue
					}

					for _, invalidValue := range invalidValues {
						if newUrl == invalidValue {
							continue
						}
					}

					// check for same domain
					if strings.HasPrefix(newUrl, "http") {
						parsedNewURL, err := url.Parse(newUrl)
						if err != nil {
							break
						}

						if domain != parsedNewURL.Host {
							continue
						}

						newUrl = parsedNewURL.Path
					}

					// check relative path and remove query params
					if strings.HasPrefix(newUrl, "/") {
						newUrl = domain + newUrl
						parsedNewURL, err := url.Parse(newUrl)
						if err != nil {
							break
						}
						newUrl = parsedNewURL.Path
					}

					// check if new url is children of target
					if checkIfChildren(newUrl, targetURL) {
						// avoid duplicates
						for _, u := range urls {
							if u == newUrl {
								continue
							}
						}

						// remove / suffix to check if it's not equal target
						newUrl = strings.TrimSuffix(newUrl, "/")
						if newUrl != targetURL {
							urls = append(urls, fmt.Sprintf("%v://%v", targetScheme, newUrl))
						}
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(htlmDoc)

	return urls, nil
}

func checkIfChildren(input string, target string) bool {
	escapedString := regexp.QuoteMeta(target)
	r := regexp.MustCompile(fmt.Sprintf(`^%v(?:\/.*|)$`, escapedString))
	return r.MatchString(input)
}
