package cli

import (
	"github.com/spf13/cobra"

	"github.com/tamnd/any-cli/kit"
)

func newGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <uri>",
		Short: "Dereference a URI: fetch and print the record",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			e, err := engineFrom()
			if err != nil {
				return err
			}
			u, err := kit.ParseURI(args[0])
			if err != nil {
				return err
			}
			env, err := e.Get(c.Context(), u)
			if err != nil {
				return err
			}
			body, hasBody := e.BodyOf(env)
			return writeEnvelope(c.OutOrStdout(), env, body, hasBody)
		},
	}
}

func newLsCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "ls <uri>",
		Short: "List the members of a collection URI",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			e, err := engineFrom()
			if err != nil {
				return err
			}
			u, err := kit.ParseURI(args[0])
			if err != nil {
				return err
			}
			envs, err := e.List(c.Context(), u, limit)
			if err != nil {
				return err
			}
			return writeStream(c.OutOrStdout(), envs)
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "n", 0, "max members to list (0 = the op default)")
	return cmd
}

func newCatCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cat <uri>",
		Short: "Print a record's body (Markdown for text, JSON otherwise)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			e, err := engineFrom()
			if err != nil {
				return err
			}
			u, err := kit.ParseURI(args[0])
			if err != nil {
				return err
			}
			body, ok, err := e.Body(c.Context(), u)
			if err != nil {
				return err
			}
			if ok {
				_, err := c.OutOrStdout().Write([]byte(body + "\n"))
				return err
			}
			// No body field: fall back to the full record.
			env, err := e.Get(c.Context(), u)
			if err != nil {
				return err
			}
			return writeJSON(c.OutOrStdout(), env)
		},
	}
}

func newLinksCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "links <uri>",
		Short: "Print the outbound link URIs (the graph edges)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			e, err := engineFrom()
			if err != nil {
				return err
			}
			u, err := kit.ParseURI(args[0])
			if err != nil {
				return err
			}
			links, err := e.Links(c.Context(), u)
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			for _, lu := range links {
				if _, err := out.Write([]byte(lu.String() + "\n")); err != nil {
					return err
				}
			}
			return nil
		},
	}
}
