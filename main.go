package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

func main() {
	c := &config{
		commands: getCommands(),
	}
	getUserInput(c)
}

func cleanInput(text string) []string {
	t := strings.ToLower(text)
	w := strings.Fields(t)
	return w
}

func getUserInput(cfg *config) {
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("Pokedex > ")
		if scanner.Scan() {
			rawInput := scanner.Text()
			cI := cleanInput(rawInput)
			commandName := cI[0]
			command, ok := cfg.commands[commandName]
			if ok {
				err := command.callback(cfg)
				if err != nil {
					fmt.Println(err)
				}
				continue
			} else {
				fmt.Println("Unknown command")
				continue
			}
		}
	}
}

type cliCommand struct {
	name     string
	desc     string
	callback func(*config) error
}

func commandExit(c *config) error {
	fmt.Println("Closing the Pokedex... Goodbye!")
	os.Exit(0)
	return nil
}

func commandHelp(c *config) error {
	fmt.Println("Welcome to the Pokedex!")
	fmt.Println("Usage:")
	fmt.Println()
	fmt.Println("help: Display a help message")
	fmt.Println("exit: Exit the Pokedex")
	return nil
}

func commandMap(cfg *config) error {
	url := "https://pokeapi.co/api/v2/location-area/"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var r reqResult
	err = json.NewDecoder(resp.Body).Decode(&r)
	if err != nil {
		return err
	}

	for _, item := range r.Results {
		fmt.Println(item.Name)
	}
	return nil
}

func getCommands() map[string]cliCommand {
	return map[string]cliCommand{
		"exit": {
			"exit",
			"exit the Pokedex",
			commandExit,
		},
		"help": {
			"help",
			"f1 for user",
			commandHelp,
		},
		"map": {
			"map",
			"list a location-area",
			commandMap,
		},
	}
}

type config struct {
	commands map[string]cliCommand
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
