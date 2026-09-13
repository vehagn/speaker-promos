package main

import (
	"fmt"
	"os"

	"github.com/vehagn/speaker-promos/internal/theme"
)

func cmdTheme(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: promo theme dump")
	}
	switch args[0] {
	case "dump":
		// Emitting the built-in theme verbatim gives a starting point to edit
		// and pass back with --theme, which beats documenting every key.
		b, err := theme.DefaultYAML()
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(b)
		return err
	default:
		return fmt.Errorf("unknown theme subcommand %q (want dump)", args[0])
	}
}
