package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
)

const maxMaterialBytes = 32 << 20

type taskMaterial struct {
	key, kind, source, name, filePath, mime, checksum string
	size                                              int64
}

func allowedMaterialURL(u *url.URL) bool {
	return u.Scheme == "https" && u.Host == "kompege.ru" && u.User == nil
}

func materialData(ctx context.Context, client *http.Client, source string) ([]byte, error) {
	if strings.HasPrefix(strings.ToLower(source), "data:") {
		if err := validateInlineImage(source); err != nil {
			return nil, err
		}
		_, encoded, _ := strings.Cut(source, ",")
		return base64.StdEncoding.DecodeString(encoded)
	}
	// Проверяем также адрес после каждого перенаправления.
	bounded := *client
	bounded.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 || !allowedMaterialURL(req.URL) {
			return fmt.Errorf("недопустимое перенаправление материала")
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil || !allowedMaterialURL(req.URL) {
		return nil, fmt.Errorf("материал за пределами разрешённого источника")
	}
	resp, err := bounded.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &taskHTTPError{Code: resp.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxMaterialBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || len(data) > maxMaterialBytes {
		return nil, fmt.Errorf("недопустимый размер материала: %d", len(data))
	}
	return data, nil
}

func materialFormat(m *taskMaterial, data []byte) (string, error) {
	mediaType, _, _ := mime.ParseMediaType(http.DetectContentType(data))
	if m.kind == db.ArtifactTypeImage {
		switch mediaType {
		case "image/png", "image/jpeg", "image/gif", "image/webp":
			m.mime = mediaType
			if mediaType == "image/jpeg" {
				return ".jpg", nil
			}
			return "." + strings.TrimPrefix(mediaType, "image/"), nil
		}
		// Сохраняем исходный SVG отдельным артефактом.
		if !strings.HasPrefix(strings.ToLower(m.source), "data:") {
			d := xml.NewDecoder(bytes.NewReader(data))
			for {
				token, err := d.Token()
				if err != nil {
					break
				}
				if start, ok := token.(xml.StartElement); ok {
					if start.Name.Local == "svg" && start.Name.Space == "http://www.w3.org/2000/svg" {
						m.kind, m.mime = db.ArtifactTypeSVG, "image/svg+xml"
						return ".svg", nil
					}
					break
				}
			}
		}
		return "", fmt.Errorf("неподдерживаемый тип изображения: %s", mediaType)
	}
	ext := strings.ToLower(filepath.Ext(m.name))
	expected := map[string]string{
		".txt": "text/plain", ".csv": "text/plain", ".pdf": "application/pdf",
		".zip": "application/zip", ".xlsx": "application/zip", ".docx": "application/zip",
		".ods": "application/zip", ".odt": "application/zip",
		".xls": "application/octet-stream", ".doc": "application/octet-stream",
	}
	if mediaType == "application/octet-stream" && (ext == ".xls" || ext == ".doc") &&
		!bytes.HasPrefix(data, []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}) {
		return "", fmt.Errorf("некорректный файл Office")
	}
	if mediaType != expected[ext] || mediaType == "" {
		return "", fmt.Errorf("неподдерживаемое вложение: %s (%s)", ext, mediaType)
	}
	m.mime = mediaType
	return ext, nil
}

func saveTaskMaterials(ctx context.Context, client *http.Client, root string, sourceID int, task *db.Task) ([]taskMaterial, error) {
	var materials []taskMaterial
	addImages := func(prefix string, value any) {
		images, _ := value.([]string)
		for i, source := range images {
			materials = append(materials, taskMaterial{key: prefix + strconv.Itoa(i+1), kind: db.ArtifactTypeImage, source: source})
		}
	}
	addImages("image_", task.Metadata["images"])
	subtasks, _ := task.Metadata["subtasks"].([]map[string]any)
	for i, sub := range subtasks {
		addImages(fmt.Sprintf("sub_%d_image_", i+1), sub["images"])
	}
	files, _ := task.Metadata["files"].([]map[string]string)
	for i, file := range files {
		materials = append(materials, taskMaterial{key: fmt.Sprintf("file_%d", i+1), kind: db.ArtifactTypeAttachment,
			source: file["url"], name: path.Base(strings.ReplaceAll(file["name"], "\\", "/"))})
	}
	if len(materials) == 0 {
		return materials, nil
	}
	if root == "" || !filepath.IsAbs(root) {
		return nil, fmt.Errorf("для скачивания материалов задайте абсолютный DATA_ROOT")
	}
	for i := range materials {
		m := &materials[i]
		if !strings.HasPrefix(strings.ToLower(m.source), "data:") {
			time.Sleep(250 * time.Millisecond)
		}
		data, err := materialData(ctx, client, m.source)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", m.key, err)
		}
		ext, err := materialFormat(m, data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", m.key, err)
		}
		hash := fmt.Sprintf("%x", sha256.Sum256(data))
		m.checksum, m.size = "sha256:"+hash, int64(len(data))
		m.filePath = path.Join("kompege", strconv.Itoa(sourceID), hash+ext)
		if err := writeMaterial(filepath.Join(root, filepath.FromSlash(m.filePath)), data); err != nil {
			return nil, err
		}
		if strings.HasPrefix(strings.ToLower(m.source), "data:") {
			m.source = ""
		}
	}
	return materials, nil
}

func writeMaterial(name string, data []byte) error {
	if existing, err := os.ReadFile(name); err == nil {
		if bytes.Equal(existing, data) {
			return nil
		}
		return fmt.Errorf("файл с ожидаемым хешем содержит другие данные: %s", name)
	} else if !os.IsNotExist(err) {
		return err
	}
	dir := filepath.Dir(name)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".download-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(tmp.Name(), name)
}

func storeTaskMaterials(ctx context.Context, store *db.DB, versionID int64, materials []taskMaterial) error {
	for _, m := range materials {
		if err := store.CreatePendingArtifact(ctx, versionID, m.kind, m.key, m.source, m.name); err != nil {
			return err
		}
		if err := store.CompleteArtifact(ctx, versionID, m.kind, m.key, m.filePath, m.mime, m.size, m.checksum); err != nil {
			return err
		}
	}
	return nil
}
