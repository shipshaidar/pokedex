package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
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
		pokedex:  make(map[string]pokemon),
	}
	repl(c)
}

type cliCommand struct {
	name     string
	desc     string
	callback func(*config, []string) error
}

type config struct {
	reqResult
	cache    *cache
	commands map[string]cliCommand
	pokedex  map[string]pokemon
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

type pokemonLoc struct {
	Id       int    `json:"id"`
	Name     string `json:"name"`
	Pokemons []struct {
		Pokemon struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"pokemon"`
	} `json:"pokemon_encounters"`
}

type pokemon struct {
	Id             int    `json:"id"`
	Name           string `json:"name"`
	BaseExperience int    `json:"base_experience"`
	Height         int    `json:"height"`
	Weight         int    `json:"weight"`
	Stats          []struct {
		BaseStat int `json:"base_stat"`
		Stat     struct {
			Name string `json:"name"`
		} `json:"stat"`
	} `json:"stats"`
	Types []struct {
		Type struct {
			Name string `json:"name"`
		} `json:"type"`
	} `json:"types"`
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

		words := cleanInput(scanner.Text())
		commandName := words[0]
		if len(commandName) == 0 {
			continue
		}
		command, ok := cfg.commands[commandName]
		if ok {
			err := command.callback(cfg, words[1:])
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
		"explore": {
			name:     "explore",
			desc:     "Show pokemons in given area",
			callback: commandExplore,
		},
		"catch": {
			name:     "catch",
			desc:     "Catches (probably) a given pokemon",
			callback: commandCatch,
		},
		"inspect": {
			name:     "inspect",
			desc:     "Show stats of a given pokemon",
			callback: commandInspect,
		},
		"pokedex": {
			name:     "pokedex",
			desc:     "Show your pokedex",
			callback: commandPokedex,
		},
	}
}

func commandHelp(cfg *config, args []string) error {
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

func commandExit(cfg *config, args []string) error {
	fmt.Println("Closing the Pokedex... Goodbye!")
	os.Exit(0)
	return nil
}

func commandMap(cfg *config, args []string) error {
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

func commandMapb(cfg *config, args []string) error {
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

func commandExplore(cfg *config, args []string) error {
	url := "https://pokeapi.co/api/v2/location-area/"
	placeUrl := url + args[0]
	res, err := getPokemonLoc(placeUrl, cfg)
	if err != nil {
		fmt.Println("invalid place")
		return err
	}
	pokemons := res.Pokemons

	fmt.Printf("Exploring %s...\n", res.Name)
	for _, item := range pokemons {
		fmt.Println(" - ", item.Pokemon.Name)
	}
	return nil
}

func commandCatch(cfg *config, args []string) error {
	pokemon := args[0]
	url := "https://pokeapi.co/api/v2/pokemon/" + pokemon
	res, err := getPokemon(url, cfg)
	if err != nil {
		return err
	}

	fmt.Printf("Throwing a Pokeball at %s...\n", pokemon)
	successRate := rand.Intn(res.BaseExperience * 2)
	if res.BaseExperience > successRate {
		fmt.Printf("%s escaped!\n", pokemon)
		return nil
	}
	fmt.Printf("%s was caught!\n", pokemon)
	cfg.pokedex[pokemon] = res
	return nil
}

func commandInspect(cfg *config, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("give me a pokemon name")
	}
	name := args[0]
	pokemon, ok := cfg.pokedex[name]

	if !ok {
		return fmt.Errorf("you have not caught that pokemon")
	}
	fmt.Printf("Name: %s\n", pokemon.Name)
	fmt.Printf("Height: %d\n", pokemon.Height)
	fmt.Printf("Weight: %d\n", pokemon.Weight)
	fmt.Println("Stats:")
	for _, s := range pokemon.Stats {
		fmt.Printf(" -%s: %d\n", s.Stat.Name, s.BaseStat)
	}
	fmt.Println("Types:")
	for _, t := range pokemon.Types {
		fmt.Printf(" -%s\n", t.Type.Name)
	}
	return nil
}

func commandPokedex(cfg *config, _ []string) error {
	if len(cfg.pokedex) < 1 {
		return fmt.Errorf("Your pokedex is empty")
	}
	fmt.Println("Your pokedex:")
	for _, p := range cfg.pokedex {
		fmt.Printf(" - %s\n", p.Name)
	}
	return nil
}

func getPokemon(url string, cfg *config) (pokemon, error) {
	var p pokemon
	data, ok := cfg.cache.Get(url)
	if ok {
		err := json.Unmarshal(data, &p)
		return p, err
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return pokemon{}, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return pokemon{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		return pokemon{}, fmt.Errorf("%d: try again\n", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return pokemon{}, err
	}
	cfg.cache.Add(url, body)
	err = json.Unmarshal(body, &p)
	if err != nil {
		return pokemon{}, err
	}
	return p, nil
}

func getPokemonLoc(url string, cfg *config) (pokemonLoc, error) {
	var p pokemonLoc
	data, ok := cfg.cache.Get(url)
	if ok {
		err := json.Unmarshal(data, &p)
		return p, err
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return pokemonLoc{}, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return pokemonLoc{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		return pokemonLoc{}, fmt.Errorf("%d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return pokemonLoc{}, err
	}
	cfg.cache.Add(url, body)

	err = json.Unmarshal(body, &p)
	if err != nil {
		return pokemonLoc{}, err
	}
	return p, nil
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
