package main

import (
        "encoding/json"
        "log"
        "net/http"
        "os"
)

type parseRequest struct {
        Query   string   `json:"query"`
        Key     []string `json:"key"`
        Private bool     `json:"private"`
}

func handleParse(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
                http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
                return
        }
        var req parseRequest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(http.StatusBadRequest)
                json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON body"})
                return
        }
        auth := AuthContext{Keys: req.Key, Private: req.Private}
        p, err := NewParser(req.Query, auth)
        var res *ParseResult
        if err == nil {
                res, err = p.Parse()
        }
        w.Header().Set("Content-Type", "application/json")
        if err != nil {
                w.WriteHeader(http.StatusBadRequest)
                json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
                return
        }
        json.NewEncoder(w).Encode(res)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.Write([]byte(`{"status":"ok"}`))
}

func main() {
        port := os.Getenv("GO_PARSER_PORT")
        if port == "" {
                port = "8081"
        }
        http.HandleFunc("/parse", handleParse)
        http.HandleFunc("/healthz", handleHealth)
        log.Printf("smap-go-parser listening on :%s", port)
        log.Fatal(http.ListenAndServe("0.0.0.0:"+port, nil))
}
