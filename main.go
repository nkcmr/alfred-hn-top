package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"runtime"
	"sync"
	"time"
)

const (
	numStories = 30
)

type hnDataService interface {
	topStories() ([]story, error)
}

type story struct {
	Id    int64   `json:"id"`
	By    string  `json:"by"`
	Title string  `json:"title"`
	Url   string  `json:"url"`
	Score int64   `json:"score"`
	Time  int64   `json:"time"`
	Kids  []int64 `json:"kids"`
}

type hndsimpl struct {
	itemfetchsem chan bool
}

func (h hndsimpl) topStories() ([]story, error) {
	log.Println("fetching top stories")
	stories := make([]story, numStories)
	tsresp, err := http.Get("https://hacker-news.firebaseio.com/v0/topstories.json")
	if err != nil {
		return stories, err
	}
	defer tsresp.Body.Close()
	rawdat, err := ioutil.ReadAll(tsresp.Body)
	if err != nil {
		return stories, err
	}
	var topIDs []int64
	err = json.Unmarshal(rawdat, &topIDs)
	if err != nil {
		return stories, err
	}
	var wg sync.WaitGroup
	fetch := func(idx int, id int64) {
		log.Printf("fetching story id %v, idx: %v", id, idx)
		defer func() { <-h.itemfetchsem }()
		defer wg.Done()
		storyresp, err := http.Get(fmt.Sprintf("https://hacker-news.firebaseio.com/v0/item/%v.json", id))
		if err != nil {
			panic(err)
		}
		defer storyresp.Body.Close()
		rawdat, err := ioutil.ReadAll(storyresp.Body)
		if err != nil {
			panic(err)
		}
		var stry story
		err = json.Unmarshal(rawdat, &stry)
		if err != nil {
			panic(err)
		}
		stories[idx] = stry
	}
	log.Println("fetching top items")
	for idx, id := range topIDs {
		if idx >= numStories {
			break
		}
		h.itemfetchsem <- true
		wg.Add(1)
		go fetch(idx, id)
	}
	wg.Wait()
	return stories, nil
}

func newHNDS() hnDataService {
	return hndsimpl{
		itemfetchsem: make(chan bool, runtime.NumCPU()),
	}
}

type hnHotFetcher struct {
	ds hnDataService
}

func (h hnHotFetcher) run() (alfredOutput, error) {
	ao := alfredOutput{Items: make([]alfredItem, numStories)}
	stories, err := h.ds.topStories()
	if err != nil {
		return ao, err
	}
	for idx, s := range stories {
		var commentsFmt string
		if len(s.Kids) == 1 {
			commentsFmt = "1 comment"
		} else {
			commentsFmt = fmt.Sprintf("%v comments", len(s.Kids))
		}
		ao.Items[idx] = alfredItem{
			Title:    s.Title,
			Subtitle: fmt.Sprintf("%v points by %s %s | %s", s.Score, s.By, timeago(s.Time), commentsFmt),
			Arg:      s.Url,
			Valid:    true,
			Mods: alfredItemModSection{
				Alt: &alfredItemMod{
					Valid:    true,
					Arg:      fmt.Sprintf("https://news.ycombinator.com/item?id=%v", s.Id),
					Subtitle: "open comments",
				},
				Cmd: nil,
			},
		}
	}
	return ao, nil
}

func timeago(ts int64) string {
	now := time.Now().Unix()
	delta := (now - ts) * int64(time.Second)
	if delta < int64(time.Hour) {
		return fmt.Sprintf("%v minutes ago", delta/60)
	} else if delta < (int64(time.Hour) * 24) {
		return fmt.Sprintf("%v hours ago", delta/(3600*int64(time.Second)))
	} else {
		dd := delta / (86400 * int64(time.Second))
		if dd == 1 {
			return "1 day ago"
		} else {
			return fmt.Sprintf("%v days ago", dd)
		}
	}
}

type alfredOutput struct {
	Items []alfredItem `json:"items"`
}

type alfredItem struct {
	Title    string               `json:"title"`
	Subtitle string               `json:"subtitle"`
	Arg      string               `json:"arg"`
	Valid    bool                 `json:"valid"`
	Mods     alfredItemModSection `json:"mods"`
}

type alfredItemModSection struct {
	Alt *alfredItemMod `json:"alt"`
	Cmd *alfredItemMod `json:"cmd,omitempty"`
}

type alfredItemMod struct {
	Valid    bool   `json:"valid"`
	Arg      string `json:"arg"`
	Subtitle string `json:"subtitle"`
}

func main() {
	log.Printf("starting hn hot fetcher (runtime: %s)", runtime.Version())
	hf := hnHotFetcher{ds: newHNDS()}
	ao, err := hf.run()
	if err != nil {
		panic(err)
	}
	emitalfredoutput(ao)
}

func emitalfredoutput(o alfredOutput) {
	dat, err := json.Marshal(o)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(dat))
}
