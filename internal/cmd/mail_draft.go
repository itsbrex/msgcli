package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/skylarbpayne/msgcli/internal/auth"
	"github.com/skylarbpayne/msgcli/internal/graph"
	"github.com/spf13/cobra"
)

var mailDraftCmd = &cobra.Command{
	Use:   "draft",
	Short: "Manage email drafts",
	Long:  `Commands to create and manage draft email messages.`,
}

var (
	mailDraftCreateTo      []string
	mailDraftCreateCc      []string
	mailDraftCreateBcc     []string
	mailDraftCreateSubject string
	mailDraftCreateBody    string
	mailDraftCreateHTML    bool
	mailDraftCreateStdin   bool
	mailDraftCreateAttach  []string
)

var mailDraftCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a draft email message",
	Long: `Create a draft email message in the Drafts folder.

The body can be provided via --body, --stdin, or interactively if neither is
specified. Use --attach <path> (repeatable) to attach files. Each attachment
must be under 3MB.`,
	RunE: runMailDraftCreate,
}

func init() {
	mailDraftCreateCmd.Flags().StringSliceVar(&mailDraftCreateTo, "to", nil, "Recipient email addresses")
	mailDraftCreateCmd.Flags().StringSliceVar(&mailDraftCreateCc, "cc", nil, "CC email addresses")
	mailDraftCreateCmd.Flags().StringSliceVar(&mailDraftCreateBcc, "bcc", nil, "BCC email addresses")
	mailDraftCreateCmd.Flags().StringVar(&mailDraftCreateSubject, "subject", "", "Email subject")
	mailDraftCreateCmd.Flags().StringVar(&mailDraftCreateBody, "body", "", "Email body")
	mailDraftCreateCmd.Flags().BoolVar(&mailDraftCreateHTML, "html", false, "Treat body as HTML")
	mailDraftCreateCmd.Flags().BoolVar(&mailDraftCreateStdin, "stdin", false, "Read body from stdin")
	mailDraftCreateCmd.Flags().StringSliceVar(&mailDraftCreateAttach, "attach", nil, "File path to attach (repeatable)")

	mailDraftCmd.AddCommand(mailDraftCreateCmd)
	mailCmd.AddCommand(mailDraftCmd)
}

func runMailDraftCreate(cmd *cobra.Command, args []string) error {
	body := mailDraftCreateBody

	if mailDraftCreateStdin {
		var sb strings.Builder
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			sb.WriteString(scanner.Text())
			sb.WriteString("\n")
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("error reading stdin: %w", err)
		}
		body = strings.TrimSuffix(sb.String(), "\n")
	} else if body == "" && !IsNoInput() {
		Infof("Enter draft body (Ctrl+D to finish, leave empty for blank draft):")
		var sb strings.Builder
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			sb.WriteString(scanner.Text())
			sb.WriteString("\n")
		}
		body = strings.TrimSuffix(sb.String(), "\n")
	}

	account, err := auth.ResolveAccount(GetAccountFlag())
	if err != nil {
		return err
	}

	client := graph.NewClient(account)
	ctx := context.Background()

	draft, err := client.CreateDraft(ctx, mailDraftCreateTo, mailDraftCreateCc, mailDraftCreateBcc, mailDraftCreateSubject, body, mailDraftCreateHTML)
	if err != nil {
		return err
	}

	for _, path := range mailDraftCreateAttach {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read attachment %q: %w", path, err)
		}
		name := filepath.Base(path)
		ctype := detectContentType(path, data)
		if err := client.AddAttachment(ctx, draft.ID, name, ctype, data); err != nil {
			return err
		}
		Infof("Attached %s (%s, %d bytes)", name, ctype, len(data))
	}

	if GetOutputFormat() == "json" {
		out, err := json.MarshalIndent(draft, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(out))
	} else {
		Infof("Draft created: %s", draft.ID)
		if draft.WebLink != "" {
			Infof("Web link: %s", draft.WebLink)
		}
	}
	return nil
}

// detectContentType returns a MIME type for the file, preferring the extension
// and falling back to content sniffing.
func detectContentType(path string, data []byte) string {
	if ct := mime.TypeByExtension(filepath.Ext(path)); ct != "" {
		return strings.SplitN(ct, ";", 2)[0]
	}
	return http.DetectContentType(data)
}
