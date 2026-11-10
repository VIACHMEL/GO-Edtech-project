package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
)

const maxInlineImageBytes = 1 << 20 // 1 МиБ на одну встроенную картинку

type TableCell struct {
	Text    string `json:"text"`
	Colspan int    `json:"colspan"`
	Rowspan int    `json:"rowspan"`
	Header  bool   `json:"header"`
}
type TableData struct {
	Rows [][]TableCell `json:"rows"`
}

func prepareFiles(files []json.RawMessage) ([]map[string]string, error) {
	result := make([]map[string]string, 0, len(files))
	base, _ := url.Parse("https://kompege.ru/")

	for i, raw := range files {
		var file struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		}
		if err := json.Unmarshal(raw, &file); err != nil {
			if err := json.Unmarshal(raw, &file.URL); err != nil {
				return nil, fmt.Errorf("files[%d]: неизвестный формат: %.100s", i, raw)
			}
		}
		if strings.TrimSpace(file.URL) == "" {
			return nil, fmt.Errorf("files[%d]: отсутствует URL", i)
		}
		ref, err := url.Parse(strings.TrimSpace(file.URL))
		if err != nil {
			return nil, fmt.Errorf("files[%d]: %w", i, err)
		}
		full := base.ResolveReference(ref)
		if full.Scheme != "http" && full.Scheme != "https" {
			return nil, fmt.Errorf("files[%d]: недопустимая ссылка", i)
		}
		name := strings.TrimSpace(file.Name)
		if name == "" {
			name = path.Base(full.Path)
		}
		result = append(result, map[string]string{"name": name, "url": full.String()})
	}
	return result, nil
}

var htmlPolicy = func() *bluemonday.Policy {
	policy := bluemonday.UGCPolicy()
	policy.AllowAttrs("colspan", "rowspan").Matching(bluemonday.Integer).OnElements("td", "th")
	policy.AllowDataURIImages()
	return policy
}()

func cleanHTML(source string) string { return strings.TrimSpace(htmlPolicy.Sanitize(source)) }

func validateInlineImage(src string) error {
	parts := strings.SplitN(src, ",", 2)
	if len(parts) != 2 {
		return fmt.Errorf("некорректная data URI")
	}
	kind := strings.ToLower(parts[0])
	switch kind {
	case "data:image/png;base64", "data:image/jpeg;base64", "data:image/gif;base64", "data:image/webp;base64":
		// Разрешённые форматы.
	default:
		return fmt.Errorf("неподдерживаемый тип встроенного изображения")
	}
	// Предварительная проверка размера до декодирования.
	if base64.StdEncoding.DecodedLen(len(parts[1])) > maxInlineImageBytes+2 {
		return fmt.Errorf("встроенное изображение превышает 1 МиБ")
	}
	data, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("неверный Base64: %w", err)
	}
	if len(data) > maxInlineImageBytes {
		return fmt.Errorf("встроенное изображение превышает 1 МиБ")
	}
	return nil
}

func prepareHTML(source string) (string, []string, error) {
	// Проверяем встроенные изображения до HTML-санации: иначе повреждённые
	// src могут исчезнуть незаметно и задание станет неполным.
	rawDoc, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return "", nil, err
	}
	rawInlineCount := 0
	var rawWalk func(*html.Node) error
	rawWalk = func(n *html.Node) error {
		if n.Type == html.ElementNode && n.Data == "img" {
			for _, attr := range n.Attr {
				if attr.Key == "src" && strings.HasPrefix(strings.ToLower(attr.Val), "data:image/") {
					rawInlineCount++
					if err := validateInlineImage(attr.Val); err != nil {
						return err
					}
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if err := rawWalk(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := rawWalk(rawDoc); err != nil {
		return "", nil, err
	}

	doc, err := html.Parse(strings.NewReader(cleanHTML(source)))
	if err != nil {
		return "", nil, err
	}
	base, _ := url.Parse("https://kompege.ru/")
	images := []string{}
	preservedInlineCount := 0

	var walk func(*html.Node) error
	walk = func(n *html.Node) error {
		if n.Type == html.ElementNode && n.Data == "img" {
			for i := range n.Attr {
				if n.Attr[i].Key != "src" {
					continue
				}
				ref, err := url.Parse(n.Attr[i].Val)
				if err != nil {
					return fmt.Errorf("URL изображения: %w", err)
				}
				if ref.Scheme == "data" {
					preservedInlineCount++
					images = append(images, n.Attr[i].Val)
					continue
				}
				full := base.ResolveReference(ref)
				if full.Scheme != "http" && full.Scheme != "https" {
					return fmt.Errorf("недопустимая схема изображения")
				}
				n.Attr[i].Val = full.String()
				images = append(images, full.String())
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(doc); err != nil {
		return "", nil, err
	}
	if rawInlineCount != preservedInlineCount {
		return "", nil, fmt.Errorf("встроенные изображения потерялись при очистке")
	}
	var result strings.Builder
	if err := html.Render(&result, doc); err != nil {
		return "", nil, err
	}
	return cleanHTML(result.String()), images, nil
}

func htmlToText(source string) string {
	tokenizer := html.NewTokenizer(strings.NewReader(source))
	var result strings.Builder
	inPre := false
	for {
		tokenType := tokenizer.Next()
		switch tokenType {
		case html.ErrorToken:
			return strings.TrimSpace(result.String())
		case html.TextToken:
			part := string(tokenizer.Text())
			if inPre || strings.TrimSpace(part) != "" {
				result.WriteString(part)
			}
		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken:
			tag, _ := tokenizer.TagName()
			switch string(tag) {
			case "td", "th":
				if tokenType == html.StartTagToken {
					result.WriteString("| ")
				} else {
					result.WriteString(" ")
				}
			case "tr":
				if tokenType == html.EndTagToken {
					result.WriteString("|\n")
				}
			case "pre":
				if tokenType == html.StartTagToken {
					inPre = true
					result.WriteString("\n")
				}
				if tokenType == html.EndTagToken {
					inPre = false
					result.WriteString("\n")
				}
			case "p", "div", "br":
				if tokenType == html.EndTagToken || string(tag) == "br" {
					result.WriteString("\n")
				}
			case "img":
				if tokenType == html.StartTagToken || tokenType == html.SelfClosingTagToken {
					result.WriteString("[изображение]")
				}
			}
		}
	}
}

func prepareSubtasks(subtasks []SubTask) ([]map[string]any, error) {
	result := make([]map[string]any, 0, len(subtasks))
	for _, sub := range subtasks {
		contentHTML, images, err := prepareHTML(sub.Text)
		if err != nil {
			return nil, fmt.Errorf("подзадание %d: %w", sub.Number, err)
		}
		tables, err := extractTables(contentHTML)
		if err != nil {
			return nil, err
		}
		var difficulty any
		if sub.Difficulty != nil {
			if name := difficultyName(*sub.Difficulty); name != "unknown" {
				difficulty = name
			}
		}
		item := map[string]any{
			"number": sub.Number, "text": contentHTML,
			"answer": strings.TrimSpace(sub.Key), "difficulty": difficulty,
		}
		if len(tables) > 0 {
			item["tables"] = tables
		}
		if len(images) > 0 {
			item["images"] = images
		}
		result = append(result, item)
	}
	return result, nil
}

func nodeText(n *html.Node) string {
	var result strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			result.WriteString(n.Data)
		}
		if n.Type == html.ElementNode && n.Data == "br" {
			result.WriteString("\n")
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return strings.TrimSpace(result.String())
}

func cellSpan(n *html.Node, name string) int {
	for _, attr := range n.Attr {
		if attr.Key == name {
			value, err := strconv.Atoi(attr.Val)
			if err == nil && value > 0 {
				return value
			}
		}
	}
	return 1
}

func extractTables(source string) ([]TableData, error) {
	doc, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return nil, err
	}
	var tables []TableData
	var findTables func(*html.Node)
	findTables = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "table" {
			var table TableData
			var findRows func(*html.Node)
			findRows = func(x *html.Node) {
				if x != n && x.Type == html.ElementNode && x.Data == "table" {
					return
				}
				if x.Type == html.ElementNode && x.Data == "tr" {
					var row []TableCell
					for cell := x.FirstChild; cell != nil; cell = cell.NextSibling {
						if cell.Type != html.ElementNode || (cell.Data != "td" && cell.Data != "th") {
							continue
						}
						row = append(row, TableCell{
							Text:    nodeText(cell),
							Colspan: cellSpan(cell, "colspan"),
							Rowspan: cellSpan(cell, "rowspan"),
							Header:  cell.Data == "th",
						})
					}
					if len(row) > 0 {
						table.Rows = append(table.Rows, row)
					}
					return
				}
				for child := x.FirstChild; child != nil; child = child.NextSibling {
					findRows(child)
				}
			}
			findRows(n)
			tables = append(tables, table)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			findTables(child)
		}
	}
	findTables(doc)
	return tables, nil
}
