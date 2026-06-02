package llmwiki

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/frederic/tgtldr/app/internal/model"
	"github.com/frederic/tgtldr/app/internal/store"
)

const maxIndexedPageBytes = 512 * 1024

type Service struct {
	store                *store.Store
	workspace            Workspace
	openAIRequestTimeout time.Duration
}

type Workspace struct {
	root string
}

type ReindexResult struct {
	Root      string `json:"root"`
	PageCount int    `json:"pageCount"`
}

func NewService(st *store.Store, root string, openAITimeout time.Duration) *Service {
	return &Service{
		store:                st,
		workspace:            NewWorkspace(root),
		openAIRequestTimeout: openAITimeout,
	}
}

func NewWorkspace(root string) Workspace {
	return Workspace{root: filepath.Clean(strings.TrimSpace(root))}
}

func (s *Service) Root() string {
	return s.workspace.root
}

func (s *Service) EnsureWorkspace() error {
	return s.workspace.Ensure()
}

func (s *Service) Reindex(ctx context.Context) (ReindexResult, error) {
	if s == nil || s.store == nil || s.store.LLMWiki == nil {
		return ReindexResult{}, fmt.Errorf("llm wiki service is not configured")
	}
	if err := s.workspace.Ensure(); err != nil {
		return ReindexResult{}, err
	}
	pages, err := s.workspace.ScanPages()
	if err != nil {
		return ReindexResult{}, err
	}
	if err := s.store.LLMWiki.ReindexPages(ctx, pages); err != nil {
		return ReindexResult{}, err
	}
	return ReindexResult{Root: s.workspace.root, PageCount: len(pages)}, nil
}

func (s *Service) SearchPages(ctx context.Context, filter store.LLMWikiPageFilter) (model.LLMWikiPageListResponse, error) {
	if s == nil || s.store == nil || s.store.LLMWiki == nil {
		return model.LLMWikiPageListResponse{}, fmt.Errorf("llm wiki service is not configured")
	}
	return s.store.LLMWiki.SearchPages(ctx, filter)
}

func (s *Service) GetPageByID(ctx context.Context, id int64) (model.LLMWikiPage, error) {
	if s == nil || s.store == nil || s.store.LLMWiki == nil {
		return model.LLMWikiPage{}, fmt.Errorf("llm wiki service is not configured")
	}
	page, err := s.store.LLMWiki.GetPageByID(ctx, id)
	if err != nil {
		return model.LLMWikiPage{}, err
	}
	content, err := s.workspace.ReadPage(page.Path)
	if err == nil {
		page.ContentText = content
	}
	return page, nil
}

func (s *Service) ListRuns(ctx context.Context, filter store.LLMWikiRunFilter) (model.LLMWikiRunListResponse, error) {
	if s == nil || s.store == nil || s.store.LLMWiki == nil {
		return model.LLMWikiRunListResponse{}, fmt.Errorf("llm wiki service is not configured")
	}
	items, err := s.store.LLMWiki.ListRuns(ctx, filter)
	if err != nil {
		return model.LLMWikiRunListResponse{}, err
	}
	return model.LLMWikiRunListResponse{Items: items}, nil
}

func (w Workspace) Ensure() error {
	if strings.TrimSpace(w.root) == "" || w.root == "." {
		return fmt.Errorf("llm wiki root is empty")
	}
	if err := os.MkdirAll(w.root, 0o700); err != nil {
		return fmt.Errorf("create llm wiki root: %w", err)
	}
	for _, item := range defaultWorkspaceFiles() {
		path, err := w.SafePath(item.path)
		if err != nil {
			return err
		}
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat llm wiki file %s: %w", item.path, err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return fmt.Errorf("create llm wiki file directory: %w", err)
		}
		if err := os.WriteFile(path, []byte(item.content), 0o600); err != nil {
			return fmt.Errorf("write llm wiki file %s: %w", item.path, err)
		}
	}
	return nil
}

func (w Workspace) ScanPages() ([]model.LLMWikiPage, error) {
	pages := make([]model.LLMWikiPage, 0)
	err := filepath.WalkDir(w.root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if shouldSkipDir(entry.Name()) && path != w.root {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat llm wiki page: %w", err)
		}
		if info.Size() > maxIndexedPageBytes {
			return nil
		}
		rel, err := filepath.Rel(w.root, path)
		if err != nil {
			return fmt.Errorf("resolve llm wiki page path: %w", err)
		}
		rel = filepath.ToSlash(rel)
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read llm wiki page %s: %w", rel, err)
		}
		pages = append(pages, ParsePage(rel, string(content), info.ModTime()))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return pages, nil
}

func (w Workspace) ReadPage(path string) (string, error) {
	safePath, err := w.SafePath(path)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(safePath)
	if err != nil {
		return "", fmt.Errorf("read llm wiki page %s: %w", path, err)
	}
	return string(content), nil
}

func (w Workspace) WritePage(path string, content string) error {
	if !strings.EqualFold(filepath.Ext(path), ".md") {
		return fmt.Errorf("llm wiki page must be a markdown file: %s", path)
	}
	safePath, err := w.SafePath(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(safePath), 0o700); err != nil {
		return fmt.Errorf("create llm wiki page directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(safePath), ".tmp-*.md")
	if err != nil {
		return fmt.Errorf("create llm wiki temp page: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write llm wiki temp page: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close llm wiki temp page: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("chmod llm wiki temp page: %w", err)
	}
	if err := os.Rename(tmpPath, safePath); err != nil {
		return fmt.Errorf("replace llm wiki page %s: %w", path, err)
	}
	return nil
}

func (w Workspace) SafePath(path string) (string, error) {
	normalized := filepath.Clean(filepath.FromSlash(strings.TrimSpace(path)))
	if normalized == "." || strings.HasPrefix(normalized, "..") || filepath.IsAbs(normalized) {
		return "", fmt.Errorf("unsafe llm wiki path: %s", path)
	}
	full := filepath.Join(w.root, normalized)
	rel, err := filepath.Rel(w.root, full)
	if err != nil {
		return "", fmt.Errorf("resolve llm wiki path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe llm wiki path: %s", path)
	}
	return full, nil
}

func ParsePage(path string, content string, updatedAt time.Time) model.LLMWikiPage {
	frontmatter, body := splitFrontmatter(content)
	page := model.LLMWikiPage{
		SpaceID:       parseInt64(frontmatter["space_id"]),
		Path:          filepath.ToSlash(strings.TrimSpace(path)),
		Title:         strings.TrimSpace(frontmatter["title"]),
		PageType:      strings.TrimSpace(frontmatter["type"]),
		ContentHash:   contentHash(content),
		ContentText:   content,
		SourceFactIDs: parseInt64List(frontmatter["source_fact_ids"]),
		UpdatedAt:     updatedAt,
	}
	if page.Title == "" {
		page.Title = firstMarkdownHeading(body)
	}
	if page.Title == "" {
		page.Title = page.Path
	}
	if page.PageType == "" {
		page.PageType = "page"
	}
	return page
}

func splitFrontmatter(content string) (map[string]string, string) {
	frontmatter := map[string]string{}
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return frontmatter, content
	}
	end := strings.Index(normalized[4:], "\n---")
	if end < 0 {
		return frontmatter, content
	}
	raw := normalized[4 : 4+end]
	body := normalized[4+end:]
	if strings.HasPrefix(body, "\n---\n") {
		body = body[5:]
	} else if strings.HasPrefix(body, "\n---") {
		body = strings.TrimPrefix(body, "\n---")
	}
	for _, line := range strings.Split(raw, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"`)
		if key != "" {
			frontmatter[key] = value
		}
	}
	return frontmatter, body
}

func firstMarkdownHeading(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		}
	}
	return ""
}

func parseInt64(value string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return n
}

func parseInt64List(value string) []int64 {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]int64, 0, len(parts))
	seen := make(map[int64]struct{}, len(parts))
	for _, part := range parts {
		n, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || n <= 0 {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out
}

func contentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func shouldSkipDir(name string) bool {
	return strings.HasPrefix(name, ".") || name == "node_modules"
}

type workspaceFile struct {
	path    string
	content string
}

func defaultWorkspaceFiles() []workspaceFile {
	return []workspaceFile{
		{
			path: "AGENTS.md",
			content: "# TGTLDR LLM Wiki\n\n" +
				"This directory is the AI-maintained long-term semantic workspace for TGTLDR.\n\n" +
				"Rules:\n" +
				"- Markdown pages are semantic memory, not runtime state.\n" +
				"- PostgreSQL remains the source of truth for Telegram messages, summaries, delivery state, and structured knowledge facts.\n" +
				"- Every important claim should cite source facts or source messages when available.\n" +
				"- Prefer updating existing pages over creating duplicate pages.\n" +
				"- Keep pages concise and organized with stable headings.\n",
		},
		{
			path: "index.md",
			content: "---\n" +
				"type: index\n" +
				"title: LLM Wiki Index\n" +
				"---\n\n" +
				"# LLM Wiki Index\n\n" +
				"This page is maintained by TGTLDR's LLM Wiki workflow.\n",
		},
		{
			path: "log.md",
			content: "---\n" +
				"type: log\n" +
				"title: LLM Wiki Log\n" +
				"---\n\n" +
				"# LLM Wiki Log\n\n",
		},
	}
}
