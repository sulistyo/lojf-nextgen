package handlers

import (
	"html/template"
	"net/http"
)

func Home(t *template.Template) http.HandlerFunc {
	view := template.Must(t.Clone())
	template.Must(view.ParseFiles("templates/pages/home.tmpl"))

	return func(w http.ResponseWriter, r *http.Request) {
		if err := view.ExecuteTemplate(w, "home.tmpl", map[string]any{"Title": "LOJF NextGen Manager"}); err != nil {
			http.Error(w, err.Error(), 500)
		}
	}
}
