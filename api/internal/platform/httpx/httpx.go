// Package httpx tem os utilitários HTTP comuns a todos os módulos: formato de
// erro, leitura de JSON e limite de corpo.
package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// MaxBodyBytes limita o corpo de toda requisição.
const MaxBodyBytes = 1 << 20

// ErrorBody é o formato de erro de toda a API:
// {"error":{"code":"snake_case","message":"..."}}.
type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WriteJSON escreve v como JSON com o status dado.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError escreve um erro no formato padrão. message pode ir para a tela:
// nunca coloque detalhes internos nela.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, ErrorBody{Error: ErrorDetail{Code: code, Message: message}})
}

// DecodeJSON lê o corpo como JSON estrito (campos desconhecidos são erro) e
// escreve a resposta 400 se falhar. Devolve false nesse caso.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			WriteError(w, http.StatusRequestEntityTooLarge, "body_too_large", "Requisição grande demais.")
			return false
		}
		WriteError(w, http.StatusBadRequest, "invalid_json", "JSON inválido.")
		return false
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		WriteError(w, http.StatusBadRequest, "invalid_json", "JSON inválido.")
		return false
	}
	return true
}

// LimitBody limita o tamanho do corpo de toda requisição.
func LimitBody(max int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, max)
			next.ServeHTTP(w, r)
		})
	}
}

// NotFound e MethodNotAllowed respondem no formato padrão.
func NotFound(w http.ResponseWriter, _ *http.Request) {
	WriteError(w, http.StatusNotFound, "not_found", "Não encontrado.")
}

func MethodNotAllowed(w http.ResponseWriter, _ *http.Request) {
	WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Método não permitido.")
}
