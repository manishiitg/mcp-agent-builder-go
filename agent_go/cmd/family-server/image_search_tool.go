package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/internal/agentsession"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/commonsimages"
)

// findImageTool saves a Commons picture next to the page being written.
// resolveDir maps the requested folder to an absolute path, or refuses it:
// the parent may name any workspace folder, the child is pinned to the
// current activity. The search itself lives in pkg/commonsimages, shared
// with the platform product.
func findImageTool(resolveDir func(requested string) (string, bool)) agentsession.Tool {
	return agentsession.Tool{
		Name:        "find_image",
		Description: commonsimages.Description,
		Category:    "family_tools",
		Params:      commonsimages.Params,
		Handler: func(ctx context.Context, args map[string]interface{}) (string, error) {
			query, _ := args["query"].(string)
			slug, _ := args["filename"].(string)
			requested, _ := args["dir"].(string)
			query = strings.TrimSpace(query)
			if query == "" {
				return "", fmt.Errorf("query is required")
			}
			absDir, ok := resolveDir(strings.TrimSpace(requested))
			if !ok {
				return "", fmt.Errorf("that folder isn't one you can write a picture into")
			}
			callCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
			defer cancel()
			img, err := commonsimages.Search(callCtx, query)
			if err != nil {
				log.Printf("[find_image] search failed for %q: %v", query, err)
				return "", err
			}
			if img == nil {
				log.Printf("[find_image] no usable picture on Commons for %q", query)
				return fmt.Sprintf(`{"status":"no_match","query":%q,"note":"Wikimedia Commons had nothing usable for this; continue without a picture, or retry with a shorter subject-only query."}`, query), nil
			}
			data, ext, err := commonsimages.Download(callCtx, img)
			if err != nil {
				return "", err
			}
			filename := commonsimages.FileName(sanitizeInboxName(slug), ext)
			if err := os.MkdirAll(absDir, 0o700); err != nil {
				return "", err
			}
			if err := os.WriteFile(filepath.Join(absDir, filename), data, 0o600); err != nil {
				return "", err
			}
			log.Printf("[find_image] saved %s (%d bytes) from %s", filename, len(data), img.PageURL)
			out, _ := json.Marshal(map[string]interface{}{
				"status":      "ok",
				"filename":    filename,
				"width":       img.Width,
				"height":      img.Height,
				"title":       img.Title,
				"attribution": commonsimages.Credit(img),
				"source":      img.PageURL,
				"embed_hint":  fmt.Sprintf("<img src=%q alt=%q>", filename, img.Title),
			})
			return string(out), nil
		},
	}
}
