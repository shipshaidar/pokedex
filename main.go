package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

func main() {
	c := &config{
		commands: getCommand(),
		cache:    newCache(5 * time.Second),
	}
	repl(c)
}

type cliCommand struct {
	name     string
	desc     string
	callback func(*config) error
}

type config struct {
	commands map[string]cliCommand
	reqResult
	cache *cache
}

type reqResult struct {
	Count    int     `json:"count"`
	Next     *string `json:"next"`
	Previous *string `json:"previous"`
	Results  []struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"results"`
}

type cache struct {
	entries map[string]cacheEntry
	mu      sync.Mutex
}

type cacheEntry struct {
	createdAt time.Time
	val       []byte
}

func newCache(interval time.Duration) *cache {
	c := cache{
		entries: make(map[string]cacheEntry),
	}
	go c.reapLoop(interval)
	return &c
}

func cleanInput(text string) []string {
	t := strings.ToLower(text)
	return strings.Fields(t)
}

func (c *cache) Add(k string, v []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[k] = cacheEntry{
		createdAt: time.Now(),
		val:       v,
	}
}

func (c *cache) Get(k string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[k]
	return entry.val, ok
}

func (c *cache) reapLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		c.mu.Lock()
		for k, v := range c.entries {
			if time.Since(v.createdAt) > interval {
				delete(c.entries, k)
			}
		}
		c.mu.Unlock()
	}
}

func repl(cfg *config) {
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("Pokedex > ")
		scanner.Scan()
		if scanner.Err() != nil {
			continue
		}
		commandName := cleanInput(scanner.Text())[0]
		if len(commandName) == 0 {
			continue
		}
		command, ok := cfg.commands[commandName]
		if ok {
			err := command.callback(cfg)
			if err != nil {
				fmt.Println(err)
			}
			continue
		}
		fmt.Println("Unknown command")
	}
}

func getCommand() map[string]cliCommand {
	return map[string]cliCommand{
		"help": {
			name:     "help",
			desc:     "Displays a help message",
			callback: commandHelp,
		},
		"exit": {
			name:     "exit",
			desc:     "Exits a Pokedex",
			callback: commandExit,
		},
		"map": {
			name:     "map",
			desc:     "Show pokemon location-areas",
			callback: commandMap,
		},
		"mapb": {
			name:     "mapb",
			desc:     "Show pokemon location-areas, but previous",
			callback: commandMapb,
		},
	}
}

func commandHelp(cfg *config) error {
	fmt.Println()
	fmt.Println("Welcome to Pokedex!")
	fmt.Println("Usage:")
	fmt.Println()
	for _, cmd := range cfg.commands {
		fmt.Printf("%s: %s\n", cmd.name, cmd.desc)
	}
	fmt.Println()
	return nil
}

func commandExit(cfg *config) error {
	fmt.Println("Closing the Pokedex... Goodbye!")
	os.Exit(0)
	return nil
}

func commandMap(cfg *config) error {
	url := "https://pokeapi.co/api/v2/location-area/"
	if cfg.Next != nil {
		url = *cfg.Next
	}
	res, err := getLocs(url, cfg)
	if err != nil {
		return err
	}
	cfg.Next = res.Next
	cfg.Previous = res.Previous
	for _, item := range res.Results {
		fmt.Println(item.Name)
	}
	return nil
}

func commandMapb(cfg *config) error {
	if cfg.Previous == nil {
		fmt.Println("you're in the first page")
		return nil
	}

	res, err := getLocs(*cfg.Previous, cfg)
	if err != nil {
		return err
	}
	cfg.Next = res.Next
	cfg.Previous = res.Previous
	for _, item := range res.Results {
		fmt.Println(item.Name)
	}
	return nil
}

func getLocs(url string, cfg *config) (reqResult, error) {
	var r reqResult
	data, ok := cfg.cache.Get(url)
	if ok {
		err := json.Unmarshal(data, &r)
		return r, err
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return reqResult{}, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return reqResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		return reqResult{}, fmt.Errorf("%d: try again.", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return reqResult{}, err
	}
	cfg.cache.Add(url, body)

	err = json.Unmarshal(body, &r)
	if err != nil {
		return reqResult{}, err
	}
	return r, nil
}
