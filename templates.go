package main

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"

	"github.com/yosssi/gohtml"
)

//go:embed *.gohtml
var gohtmlTemplates embed.FS

func formatTemplate(filename string, templateData interface{}, output *[]byte) error {
	tpl, err := template.ParseFS(gohtmlTemplates, filename)
	if err != nil {
		return fmt.Errorf("could not parse template %s: %w", filename, err)
	}

	buf := &bytes.Buffer{}
	if err := tpl.Execute(buf, templateData); err != nil {
		return fmt.Errorf("could not execute template: %w", err)
	}

	*output = gohtml.FormatBytes(buf.Bytes())

	return nil
}
