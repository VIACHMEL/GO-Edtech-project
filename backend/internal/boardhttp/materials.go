package boardhttp

import (
	"fmt"
	"html"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
	nethtml "golang.org/x/net/html"
)

type materialLink struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Inline bool   `json:"inline"`
}

type TaskPresentation struct {
	HTML      string         `json:"condition_html,omitempty"`
	Materials []materialLink `json:"materials,omitempty"`
}

func raster(a db.ConditionArtifact) bool {
	switch a.MIME {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return a.Kind == db.ArtifactTypeImage
	default:
		return false
	}
}

// PresentCondition оставляет только разметку условия и локальные ссылки из БД.
func PresentCondition(condition db.TaskCondition, urlPrefix string) TaskPresentation {
	var result TaskPresentation
	images := make(map[string]materialLink)
	for _, artifact := range condition.Artifacts {
		name := artifact.Name
		if name == "" {
			name = "Рисунок " + strings.TrimPrefix(artifact.Key, "image_")
		}
		link := materialLink{Name: name, URL: urlPrefix + strconv.FormatInt(artifact.ID, 10), Inline: raster(artifact)}
		result.Materials = append(result.Materials, link)
		if link.Inline {
			images[artifact.Key] = link
		}
	}
	if condition.HTML == "" {
		return result
	}
	document, err := nethtml.Parse(strings.NewReader(condition.HTML))
	if err != nil {
		return result
	}
	var out strings.Builder
	imageNumber := 0
	var render func(*nethtml.Node)
	render = func(n *nethtml.Node) {
		if n.Type == nethtml.TextNode {
			out.WriteString(html.EscapeString(n.Data))
			return
		}
		tag := ""
		if n.Type == nethtml.ElementNode {
			switch n.Data {
			case "head", "script", "style", "iframe", "object", "template", "svg", "math":
				return
			case "img":
				imageNumber++
				key := "image_" + strconv.Itoa(imageNumber)
				if image, ok := images[key]; ok {
					alt := image.Name
					for _, attribute := range n.Attr {
						if attribute.Key == "alt" && attribute.Val != "" {
							alt = attribute.Val
						}
					}
					fmt.Fprintf(&out, `<img src="%s" alt="%s" loading="lazy">`, html.EscapeString(image.URL), html.EscapeString(alt))
				} else {
					out.WriteString("<span>[рисунок см. в материалах]</span>")
				}
				return
			case "br", "hr":
				out.WriteString("<" + n.Data + ">")
				return
			case "p", "div", "strong", "b", "em", "i", "u", "sup", "sub", "s", "ul", "ol", "li",
				"pre", "code", "blockquote", "table", "caption", "thead", "tbody", "tfoot", "tr", "th", "td":
				tag = n.Data
			}
		}
		if tag != "" {
			out.WriteString("<" + tag)
			if tag == "td" || tag == "th" {
				for _, attribute := range n.Attr {
					if attribute.Key == "colspan" || attribute.Key == "rowspan" {
						if span, err := strconv.Atoi(attribute.Val); err == nil && span > 0 && span <= 100 {
							fmt.Fprintf(&out, ` %s="%d"`, attribute.Key, span)
						}
					}
				}
			}
			out.WriteByte('>')
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			render(child)
		}
		if tag != "" {
			out.WriteString("</" + tag + ">")
		}
	}
	render(document)
	result.HTML = strings.TrimSpace(out.String())
	return result
}

func (h *Handler) conditionMaterial(w http.ResponseWriter, r *http.Request) {
	actor, id, ok := h.endpoint(w, r)
	if !ok {
		return
	}
	version := id
	if rawItem := r.PathValue("item_id"); rawItem != "" {
		item, err := strconv.ParseInt(rawItem, 10, 64)
		if err != nil || item <= 0 {
			http.NotFound(w, r)
			return
		}
		version, err = h.store.AttemptItemVersion(r.Context(), actor, id, item)
		if err != nil {
			handleError(w, err)
			return
		}
	} else {
		user, err := h.store.GetUser(r.Context(), actor)
		if err != nil || user.Role != "teacher" {
			http.NotFound(w, r)
			return
		}
	}
	artifactID, err := strconv.ParseInt(r.PathValue("artifact_id"), 10, 64)
	if err != nil || artifactID <= 0 {
		http.NotFound(w, r)
		return
	}
	conditions, err := h.store.TaskConditions(r.Context(), []int64{version})
	if err != nil {
		handleError(w, err)
		return
	}
	for _, artifact := range conditions[version].Artifacts {
		if artifact.ID == artifactID {
			h.serveMaterial(w, r, artifact)
			return
		}
	}
	http.NotFound(w, r)
}

func (h *Handler) serveMaterial(w http.ResponseWriter, r *http.Request, artifact db.ConditionArtifact) {
	local := filepath.FromSlash(artifact.Path)
	if !filepath.IsLocal(local) {
		http.NotFound(w, r)
		return
	}
	root, err := os.OpenRoot(h.dataRoot)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer root.Close()
	file, err := root.Open(local)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	name := filepath.Base(strings.ReplaceAll(artifact.Name, "\\", "/"))
	if name == "." || name == "/" {
		name = filepath.Base(local)
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	if raster(artifact) {
		w.Header().Set("Content-Type", artifact.MIME)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	}
	http.ServeContent(w, r, name, info.ModTime(), file)
}
