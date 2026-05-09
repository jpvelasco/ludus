package setup

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

var scanner *bufio.Scanner

func init() {
	scanner = bufio.NewScanner(os.Stdin)
}

// prompt displays a question with an optional default and reads user input.
func prompt(question, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", question, defaultVal)
	} else {
		fmt.Printf("%s: ", question)
	}
	scanner.Scan()
	answer := strings.TrimSpace(scanner.Text())
	if answer == "" {
		return defaultVal
	}
	return answer
}

// promptChoice displays a question with numbered choices and returns the selected value.
func promptChoice(question string, choices []string, defaultIdx int) string {
	fmt.Println(question)
	for i, c := range choices {
		marker := "  "
		if i == defaultIdx {
			marker = "* "
		}
		fmt.Printf("  %s%d) %s\n", marker, i+1, c)
	}
	fmt.Printf("Choice [%d]: ", defaultIdx+1)
	scanner.Scan()
	answer := strings.TrimSpace(scanner.Text())
	if answer == "" {
		return choices[defaultIdx]
	}

	for i, c := range choices {
		if answer == fmt.Sprintf("%d", i+1) || strings.EqualFold(answer, c) {
			return c
		}
	}
	return choices[defaultIdx]
}

// confirm asks a yes/no question.
func confirm(question string) bool {
	fmt.Printf("%s [Y/n]: ", question)
	scanner.Scan()
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return answer == "" || answer == "y" || answer == "yes"
}
