package server

import (
	"fmt"
	"html/template"
	"sync"
)

// directoryTemplate provides HTML template for directory listings.
var directoryTemplate = sync.OnceValue(func() *template.Template {
	tmpl := `<!DOCTYPE html>
<html>
<head>
    <title>Recordings - {{.Path}}</title>
    <style>
        body { font-family: monospace; margin: 20px; }
        h1 { font-size: 24px; }
        table { border-collapse: collapse; width: 100%; max-width: 1000px; }
        th { text-align: left; border-bottom: 1px solid #ddd; padding: 8px; }
        td { padding: 8px; }
        tr:hover { background-color: #f5f5f5; }
        a { text-decoration: none; color: #0066cc; }
        a:hover { text-decoration: underline; }
        .size { text-align: right; }
        .time { color: #666; }
    </style>
</head>
<body>
    <h1>Index of /recordings{{.Path}}</h1>
    <table>
        <thead>
            <tr>
                <th>Name</th>
                <th>Size</th>
                <th>Modified</th>
            </tr>
        </thead>
        <tbody>
            {{range .Files}}
            <tr>
                <td><a href="{{.URL}}">{{.Name}}</a></td>
                <td class="size">{{.Size}}</td>
                <td class="time">{{.ModTime}}</td>
            </tr>
            {{end}}
        </tbody>
    </table>
</body>
</html>`

	t, err := template.New("listing").Parse(tmpl)
	if err != nil {
		panic(fmt.Sprintf("template parse error: %v", err))
	}
	return t
})
