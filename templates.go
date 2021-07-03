package main

import (
	"bytes"
	"fmt"
	"html/template"

	"github.com/yosssi/gohtml"
)

func formatTemplate(filename string, templateData interface{}, output *[]byte) error {
	tpl, err := template.ParseFiles(filename)
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
