package main

import (
	"errors"
	"fmt"
	"log"
	"log/slog"
	"strings"
	"sync"

	"github.com/gocolly/colly"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func getSeed() []string {
	seeds := make([]string, 27)
	seeds[0] = "0-9"
	for i := 1; i <= 26; i++ {
		seeds[i] = string(byte('a' + i - 1))
	}
	return seeds
}

const baseURL = "https://dictionary.cambridge.org/browse/english/"

type URL string

// Each character has vocabs devided by range
// Return url to those indexes for each character
func ExtractIndex(ch string) <-chan URL {
	url := baseURL + ch
	slog.Info("Extract indexes", "url", url)
	c := colly.NewCollector()
	c.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"
	indexes := make(chan URL)

	var link string
	indexSelector := ".hlh32.hdb.dil.tcbd"
	c.OnHTML(indexSelector, func(e *colly.HTMLElement) {
		link = e.Attr("href")
		indexes <- URL(link)
	})

	c.OnError(func(_ *colly.Response, err error) {
		slog.Error(err.Error())
	})

	go func() {
		if err := c.Visit(url); err != nil {
			slog.Error("Failed when get index", "character", ch, "error", err.Error())
		}
		close(indexes)
	}()

	return indexes
}

// Get the URL to look up words
func CrawlVocabURL(index URL) <-chan URL {
	vocabUrls := make(chan URL)
	c := colly.NewCollector()
	slog.Info("Start crawl urls", "index", index)

	c.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"

	var link string
	wordUrlSelector := ".tc-bd"
	c.OnHTML(wordUrlSelector, func(e *colly.HTMLElement) {
		link = e.Attr("href")
		if link != "" {
			vocabUrls <- URL(fmt.Sprintf("https://dictionary.cambridge.org%s", link))
		}
	})

	c.OnError(func(_ *colly.Response, err error) {
		slog.Error(err.Error())
	})

	go func() {
		if err := c.Visit(string(index)); err != nil {
			slog.Error("Failed when crawl", "index", index, "error", err.Error())
		}
		close(vocabUrls)
	}()

	return vocabUrls
}

type CrawlVocabResponse struct {
	vocab *Vocab
	err   error
}

func (resp CrawlVocabResponse) String() string {
	if resp.err != nil {
		return fmt.Sprintf("Faild when crawl, error: %s", resp.err.Error())
	}
	return fmt.Sprintf("Result of crawl, vocab: %s", resp.vocab)
}

func CrawlVocab(vocabUrl URL, ch chan<- CrawlVocabResponse) {
	vocab := Vocab{
		Level: defaultLevel,
	}
	c := colly.NewCollector()
	c.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"

	slog.Info("Start crawl vocab", "vocabURL", vocabUrl)
	wordSelector := ".hw.dhw"
	hasFoundWord := false
	c.OnHTML(wordSelector, func(e *colly.HTMLElement) {
		vocab.Word = e.Text
		hasFoundWord = true
	})

	firstDefinitionFound := false
	definitionSelector := ".def.ddef_d.db"
	c.OnHTML(definitionSelector, func(e *colly.HTMLElement) {
		if !firstDefinitionFound {
			firstDefinitionFound = true
			text := strings.TrimSpace(e.Text)
			if text[len(text)-1] == ':' {
				vocab.Definition = text[:len(text)-1]
			} else {
				vocab.Definition = text
			}
		}
	})

	levelSelector := ".epp-xref.dxref"
	c.OnHTML(levelSelector, func(e *colly.HTMLElement) {
		if e.Index == 0 {
			vocab.Level = convertStringToLevel(strings.TrimSpace(e.Text))
		}
	})

	exampleSelector := ".eg.deg"
	c.OnHTML(exampleSelector, func(e *colly.HTMLElement) {
		vocab.ExampleUsage = e.Text
	})

	if err := c.Visit(string(vocabUrl)); err != nil {
		slog.Error("Failed when crawl vocab", "vocabURL", vocabUrl, "error", err.Error())
	}

	if !hasFoundWord {
		ch <- CrawlVocabResponse{
			vocab: nil,
			err:   ErrWordNotFound,
		}
		return
	}

	if !firstDefinitionFound {
		ch <- CrawlVocabResponse{
			vocab: nil,
			err:   ErrDefinitionNotFound,
		}
		return
	}

	ch <- CrawlVocabResponse{&vocab, nil}

}

var ErrWordNotFound = errors.New("failed to find word")
var ErrDefinitionNotFound = errors.New("failed to find definition")

func CrawlHandle(vocabUrls <-chan URL) <-chan CrawlVocabResponse {
	responses := make(chan CrawlVocabResponse)

	go func() {
		var wg sync.WaitGroup
		for vocabUrl := range vocabUrls {
			wg.Add(1)
			go func(v URL) {
				CrawlVocab(v, responses)
				wg.Done()
			}(vocabUrl)

		}
		wg.Wait()
		close(responses)
	}()

	return responses
}

func main() {
	seeds := getSeed()

	for _, seed := range seeds[2:] {
		indexes := ExtractIndex(seed)
		var wg sync.WaitGroup
		for index := range indexes {
			vocabUrls := CrawlVocabURL(index)
			wg.Add(1)
			go func() {
				defer wg.Done()
				responses := CrawlHandle(vocabUrls)

				for response := range responses {
					fmt.Println(response)
				}
			}()
		}
		wg.Wait()
		break
	}

}

type Vocab struct {
	Word         string
	Definition   string
	Level        Level  `json:"level,omitempty"`
	ExampleUsage string `json:"example_usage,omitempty"`
}

func (v Vocab) String() string {
	return fmt.Sprintf("Word: %s, Definition: %s, ExampleUsage: %s", v.Word, v.Definition, v.ExampleUsage)
}

type Level int

const (
	A1 Level = iota
	A2
	B1
	B2
	C1
	C2
)

const defaultLevel = B2

func convertStringToLevel(l string) Level {
	levels := []string{"A1", "A2", "B1", "B2", "C1", "C2"}
	for i, level := range levels {
		if l == level {
			return Level(i)
		}
	}
	return Level(-1) // Treat -1 as error
}
