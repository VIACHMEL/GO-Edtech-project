package main

import (
	"encoding/json"
	"fmt"
	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
	"net/url"
	"path"
	"strconv"
	"strings"
)

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
			// Поддерживаем также JSON-строку с URL
			if err := json.Unmarshal(raw, &file.URL); err != nil {
				return nil, fmt.Errorf("files[%d]: неизвестный формат: %s", i, raw)
			}
		}

		if strings.TrimSpace(file.URL) == "" {
			return nil, fmt.Errorf("files[%d]: отсутствует URL: %s", i, raw)
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

		result = append(result, map[string]string{
			"name": name,
			"url":  full.String(),
		})
	}

	return result, nil
}

func prepareHTML(source string) (string, []string, error) {
	doc, err := html.Parse(strings.NewReader(cleanHTML(source)))
	if err != nil {
		return "", nil, err
	}

	base, _ := url.Parse("https://kompege.ru/")
	images := []string{}

	var walk func(*html.Node)

	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "img" {
			for i := range n.Attr {
				if n.Attr[i].Key != "src" {
					continue
				}

				ref, err := url.Parse(n.Attr[i].Val)
				if err != nil {
					continue
				}

				full := base.ResolveReference(ref)

				if full.Scheme != "http" && full.Scheme != "https" {
					continue
				}

				n.Attr[i].Val = full.String()
				images = append(images, full.String())
			}
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}

	walk(doc)

	var result strings.Builder
	if err := html.Render(&result, doc); err != nil {
		return "", nil, err
	}

	return cleanHTML(result.String()), images, nil
}

var htmlPolicy = func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()

	p.AllowAttrs("colspan", "rowspan").
		Matching(bluemonday.Integer).
		OnElements("td", "th")

	return p
}()

func cleanHTML(source string) string {
	return strings.TrimSpace(htmlPolicy.Sanitize(source))
}
func htmlToText(source string) string {
	tokenizator := html.NewTokenizer(strings.NewReader(source))
	var result strings.Builder

	for {
		tokenType := tokenizator.Next()
		switch tokenType {
		case html.ErrorToken:
			return strings.TrimSpace(result.String())
		case html.TextToken:
			part := string(tokenizator.Text())
			if strings.TrimSpace(part) != "" {
				result.WriteString(part)
			}
		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken:
			tag, _ := tokenizator.TagName()

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

			case "p", "div", "br", "pre":
				if tokenType == html.EndTagToken || string(tag) == "br" {
					result.WriteString("\n")
				}

			case "img":
				if tokenType == html.StartTagToken {
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

		var difficulty any = nil

		if sub.Difficulty != nil {
			name := difficultyName(*sub.Difficulty)
			if name != "unknown" {
				difficulty = name
			}
		}

		item := map[string]any{
			"number":     sub.Number,
			"text":       contentHTML,
			"answer":     strings.TrimSpace(sub.Key),
			"difficulty": difficulty,
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
				// Не смешиваем строки вложенных таблиц.
				if x != n && x.Type == html.ElementNode && x.Data == "table" {
					return
				}

				if x.Type == html.ElementNode && x.Data == "tr" {
					var row []TableCell

					for cell := x.FirstChild; cell != nil; cell = cell.NextSibling {
						if cell.Type != html.ElementNode {
							continue
						}

						if cell.Data != "td" && cell.Data != "th" {
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
