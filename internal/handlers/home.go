package handlers

import (
	"html/template"
	"net/http"
)

func Home(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		view, err := t.Clone()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if _, err := view.ParseFiles("templates/pages/home.tmpl"); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		data := map[string]any{"Title": "LOJF NextGen Manager"}
		if err := view.ExecuteTemplate(w, "home.tmpl", data); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
}
