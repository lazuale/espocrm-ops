package result

import (
	"encoding/json"
	"fmt"
	"io"
)

func Render(w io.Writer, res Result, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}

	if res.OK {
		if res.Message != "" {
			if _, err := fmt.Fprintln(w, res.Message); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintln(w, "ok"); err != nil {
				return err
			}
		}

		for _, warning := range res.Warnings {
			if _, err := fmt.Fprintf(w, "WARNING: %s\n", warning); err != nil {
				return err
			}
		}
		return nil
	}

	if res.Error != nil {
		_, err := fmt.Fprintf(w, "ERROR [%s]: %s\n", res.Error.Code, res.Error.Message)
		return err
	}

	_, err := fmt.Fprintln(w, "ERROR")
	return err
}
