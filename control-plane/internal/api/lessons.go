package api

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/mikelm20/learn-platform/control-plane/internal/apitypes"
	"gopkg.in/yaml.v3"
)

type lessonsHandler struct {
	deps Deps
}

// lessonFile mirrors the relevant fields of the YAML. Unknown fields
// are tolerated; YAML unmarshal ignores them.
type lessonFile struct {
	ID               string  `yaml:"id" json:"id"`
	ModuleNumber     int     `yaml:"module_number" json:"module_number"`
	Language         string  `yaml:"language" json:"language"`
	Title            string  `yaml:"title" json:"title"`
	Subtitle         *string `yaml:"subtitle" json:"subtitle,omitempty"`
	Description      *string `yaml:"description" json:"description,omitempty"`
	EstimatedMinutes *int    `yaml:"estimated_minutes" json:"estimated_minutes,omitempty"`

	// We preserve raw steps + the rest so GET /lessons/:id returns the full
	// parsed document without us having to model every field again.
	Raw map[string]any `yaml:"-" json:"-"`
}

var (
	lessonCacheMu sync.Mutex
	lessonCache   map[string]*lessonFile // keyed by lesson ID + "." + lang
)

func (h *lessonsHandler) List(w http.ResponseWriter, r *http.Request) {
	all, err := h.loadAll()
	if err != nil {
		h.deps.Logger.Error("load lessons", "err", err)
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrInternal, "cannot load lessons")
		return
	}
	// Optional ?lang= filter.
	lang := r.URL.Query().Get("lang")
	summaries := make([]apitypes.LessonSummary, 0, len(all))
	for _, l := range all {
		if lang != "" && l.Language != lang {
			continue
		}
		s := apitypes.LessonSummary{
			ID:               l.ID,
			ModuleNumber:     l.ModuleNumber,
			Language:         apitypes.Lang(l.Language),
			Title:            l.Title,
			Subtitle:         l.Subtitle,
			EstimatedMinutes: l.EstimatedMinutes,
		}
		summaries = append(summaries, s)
	}
	sort.SliceStable(summaries, func(i, j int) bool {
		if summaries[i].ModuleNumber != summaries[j].ModuleNumber {
			return summaries[i].ModuleNumber < summaries[j].ModuleNumber
		}
		return summaries[i].ID < summaries[j].ID
	})
	writeJSON(w, http.StatusOK, apitypes.ListLessonsResponse{Lessons: summaries})
}

// Get handles GET /lessons/{id}. Optional ?lang=; defaults to es when the id
// doesn't already encode a language.
func (h *lessonsHandler) Get(w http.ResponseWriter, r *http.Request) {
	idParam := chi.URLParam(r, "id")
	if idParam == "" {
		apiError(w, r, http.StatusBadRequest, apitypes.ErrBadRequest, "missing id")
		return
	}
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = "es"
	}

	l, err := h.loadOne(idParam, lang)
	if err != nil {
		if os.IsNotExist(err) {
			apiError(w, r, http.StatusNotFound, apitypes.ErrNotFound, "lesson not found")
			return
		}
		h.deps.Logger.Error("load lesson", "err", err, "id", idParam, "lang", lang)
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrInternal, "cannot load lesson")
		return
	}
	// Serve the full parsed document as JSON.
	writeJSON(w, http.StatusOK, l.Raw)
}

func (h *lessonsHandler) loadAll() ([]*lessonFile, error) {
	lessonCacheMu.Lock()
	defer lessonCacheMu.Unlock()
	if lessonCache != nil {
		out := make([]*lessonFile, 0, len(lessonCache))
		for _, v := range lessonCache {
			out = append(out, v)
		}
		return out, nil
	}
	cache := make(map[string]*lessonFile)
	entries, err := os.ReadDir(h.deps.LessonsDir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yml") {
			continue
		}
		l, err := parseLessonFile(filepath.Join(h.deps.LessonsDir, e.Name()))
		if err != nil {
			continue
		}
		cache[l.ID+"."+l.Language] = l
	}
	lessonCache = cache
	out := make([]*lessonFile, 0, len(cache))
	for _, v := range cache {
		out = append(out, v)
	}
	return out, nil
}

func (h *lessonsHandler) loadOne(id, lang string) (*lessonFile, error) {
	// Try exact filename id.lang.yml first.
	candidates := []string{
		filepath.Join(h.deps.LessonsDir, id+"."+lang+".yml"),
		filepath.Join(h.deps.LessonsDir, id+".yml"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return parseLessonFile(p)
		}
	}
	// Fall back to searching loaded cache.
	all, err := h.loadAll()
	if err != nil {
		return nil, err
	}
	for _, l := range all {
		if l.ID == id && l.Language == lang {
			return l, nil
		}
	}
	return nil, os.ErrNotExist
}

func parseLessonFile(path string) (*lessonFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	l := &lessonFile{Raw: raw}
	if v, ok := raw["id"].(string); ok {
		l.ID = v
	}
	if v, ok := raw["module_number"].(int); ok {
		l.ModuleNumber = v
	}
	if v, ok := raw["language"].(string); ok {
		l.Language = v
	}
	if v, ok := raw["title"].(string); ok {
		l.Title = v
	}
	if v, ok := raw["subtitle"].(string); ok {
		l.Subtitle = &v
	}
	if v, ok := raw["description"].(string); ok {
		l.Description = &v
	}
	if v, ok := raw["estimated_minutes"].(int); ok {
		l.EstimatedMinutes = &v
	}
	// If language is absent (hello-claude.yml), infer "es" as a safe default.
	if l.Language == "" {
		l.Language = "es"
	}
	return l, nil
}
