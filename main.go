package main

import (
	"bufio"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"gopkg.in/yaml.v2"
)

// Config :
type Config struct {
	Time     time.Duration
	Parallel int
	Rules    map[string]string
}

func setUpLogFile() *os.File {
	f, err := os.OpenFile("requests.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	log.SetOutput(f)
	return f
}

func main() {
	defer setUpLogFile().Close()
	config := getConfig()
	urls := getUrls()

	runPeriodicalChecks(urls, config)
	blockForever()
}

func getConfig() Config {
	config := Config{}
	dat, err := ioutil.ReadFile("config.yaml")
	if err != nil {
		log.Fatal(err)
	}
	err = yaml.Unmarshal(dat, &config)
	if err != nil {
		panic(err)
	}
	return config
}

func getUrls() (urls []string) {
	fileHandle, err := os.Open("url.txt")
	if err != nil {
		panic(err)
	}
	defer fileHandle.Close()
	fileScanner := bufio.NewScanner(fileHandle)
	for fileScanner.Scan() {
		urls = append(urls, fileScanner.Text())
	}
	return urls
}

func blockForever() {
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		os.Exit(1)
	}()

	for {
		time.Sleep(1 * time.Hour)
	}
}

func runPeriodicalChecks(urls []string, config Config) {
	// run first checks
	runChecks(urls, config)
	ticker := time.NewTicker(config.Time * time.Minute)
	go func() {
		for {
			select {
			case <-ticker.C:
				// run every N minutes
				runChecks(urls, config)
			}
		}
	}()
}

func runChecks(urls []string, config Config) {
	var url string
	var wg sync.WaitGroup
	// config.Parallel to throttle no.of parallel request, so depending on system, network capabilities we can increase and decrease no.of parallel requests
	chanParallel := make(chan bool, config.Parallel)
	for _, url = range urls {
		wg.Add(1)
		chanParallel <- true
		go func(url string) {
			checkURL(url, config.Rules[url], &wg)
			time.Sleep(1 * time.Second)
			<-chanParallel
		}(url)
	}
	wg.Wait()
}

func checkURL(url string, match string, wg *sync.WaitGroup) {
	defer wg.Done()
	if !strings.HasPrefix(url, "http") {
		url = "http://" + url
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("%s %s\n", url, err.Error())
		return
	}
	client := &http.Client{Timeout: time.Duration(10 * time.Second)}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("%s %s %v\n", url, err.Error(), time.Since(start))
		return
	}
	defer resp.Body.Close()
	elapsed := time.Since(start)
	if resp.StatusCode != 200 {
		log.Printf("%s %s %v\n", url, resp.Status, elapsed)
		return
	}

	html, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("%s %s %v %s\n", url, resp.Status, elapsed, err.Error())
		return
	}
	log.Printf("%s %s %v\n", url, resp.Status, elapsed)
	if match != "" && strings.Contains(string(html), match) {
		log.Printf("content requirement for %s is satisfied\n", url)
	} else {
		log.Printf("content requirement for %s isn't satisfied\n", url)
	}
}
