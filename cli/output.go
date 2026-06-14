package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/tamnd/any-cli/kit"
)

// writeJSON prints v as indented JSON followed by a newline.
func writeJSON(w io.Writer, v any) error {
	blob, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", blob)
	return err
}

// writeEnvelope prints one record: its Markdown body when -o md is set and the
// record has one, otherwise the full JSON envelope.
func writeEnvelope(w io.Writer, env kit.Envelope, body string, hasBody bool) error {
	if flagOutput == "md" && hasBody {
		_, err := fmt.Fprintf(w, "%s\n", body)
		return err
	}
	return writeJSON(w, env)
}

// writeStream prints a slice of envelopes as a JSON array (the lossless default)
// or, for -o md, each record's id on its own line as a quick index.
func writeStream(w io.Writer, envs []kit.Envelope) error {
	if flagOutput == "md" {
		for _, env := range envs {
			if _, err := fmt.Fprintln(w, env.ID); err != nil {
				return err
			}
		}
		return nil
	}
	return writeJSON(w, envs)
}
