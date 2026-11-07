package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/app"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/auth"
)

func runCommand(ctx context.Context, config app.Config, logger *slog.Logger, args []string, input *os.File, output io.Writer) error {
	if len(args) < 2 || args[0] != "admin" || args[1] != "reset-password" {
		return errors.New("usage: mediahub admin reset-password [--password-stdin]; stop the application first and use its configured database")
	}
	flags := flag.NewFlagSet("admin reset-password", flag.ContinueOnError)
	flags.SetOutput(output)
	fromStdin := flags.Bool("password-stdin", false, "read one password from standard input; never pass passwords as arguments")
	if err := flags.Parse(args[2:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("password arguments are forbidden; use hidden terminal input or --password-stdin")
	}
	password, err := readNewPassword(input, output, *fromStdin)
	if err != nil {
		return err
	}
	if err := app.ResetAdministratorPassword(ctx, config, logger, password); err != nil {
		if errors.Is(err, auth.ErrSetupRequired) {
			return errors.New("administrator is not initialized; recovery never creates an account")
		}
		return err
	}
	_, err = fmt.Fprintln(output, "Administrator password updated. All prior sessions have been revoked. Settings are preserved; restart the application.")
	return err
}

func readNewPassword(input *os.File, output io.Writer, fromStdin bool) (string, error) {
	if fromStdin {
		// Read at most the ASCII password limit plus CRLF and one overflow byte.
		data, err := io.ReadAll(io.LimitReader(input, int64(auth.MaxPasswordLength+3)))
		if err != nil {
			return "", fmt.Errorf("read password input: %w", err)
		}
		password := strings.TrimSuffix(string(data), "\n")
		if len(data) > 0 && data[len(data)-1] == '\n' {
			password = strings.TrimSuffix(password, "\r")
		}
		if strings.ContainsAny(password, "\r\n") || len(data) >= auth.MaxPasswordLength+3 {
			return "", errors.New("provide exactly one password of 8–32 printable non-space ASCII characters on standard input")
		}
		if err := auth.ValidatePassword(password); err != nil {
			return "", errors.New("password must contain 8–32 printable non-space ASCII characters (letters, digits or ASCII symbols)")
		}
		return password, nil
	}
	if !term.IsTerminal(int(input.Fd())) {
		return "", errors.New("hidden terminal input is unavailable; explicitly use --password-stdin for piped/container input")
	}
	password, err := promptPassword(input, output, "New administrator password: ")
	if err != nil {
		return "", err
	}
	confirmation, err := promptPassword(input, output, "Confirm password: ")
	if err != nil {
		return "", err
	}
	if password != confirmation {
		return "", errors.New("password confirmation does not match")
	}
	if err := auth.ValidatePassword(password); err != nil {
		return "", errors.New("password must contain 8–32 printable non-space ASCII characters (letters, digits or ASCII symbols)")
	}
	return password, nil
}

func promptPassword(input *os.File, output io.Writer, prompt string) (string, error) {
	if _, err := fmt.Fprint(output, prompt); err != nil {
		return "", err
	}
	password, err := term.ReadPassword(int(input.Fd()))
	if _, writeErr := fmt.Fprintln(output); writeErr != nil {
		return "", errors.Join(err, writeErr)
	}
	if err != nil {
		return "", fmt.Errorf("read hidden password: %w", err)
	}
	return string(password), nil
}
