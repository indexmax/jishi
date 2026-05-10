package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultChunkSize int64 = 4 * 1024 * 1024

type uploadRecord struct {
	FileHash    string       `json:"fileHash"`
	FileName    string       `json:"fileName"`
	FileSize    int64        `json:"fileSize"`
	ChunkSize   int64        `json:"chunkSize"`
	TotalChunks int          `json:"totalChunks"`
	Uploaded    map[int]bool `json:"uploaded"`
	Status      string       `json:"status"`
	Path        string       `json:"path,omitempty"`
	UpdatedAt   time.Time    `json:"updatedAt"`
}

type metadataStore struct {
	mu      sync.Mutex
	dbPath  string
	records map[string]*uploadRecord
}

func newMetadataStore(dbPath string) (*metadataStore, error) {
	store := &metadataStore{
		dbPath:  dbPath,
		records: map[string]*uploadRecord{},
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *metadataStore) load() error {
	data, err := os.ReadFile(s.dbPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}
	return json.Unmarshal(data, &s.records)
}

func (s *metadataStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.dbPath), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.records, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.dbPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.dbPath)
}

type uploadServer struct {
	store    *metadataStore
	chunkDir string
	fileDir  string
	page     *template.Template
}

func newUploadServer(dataDir string) (*uploadServer, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}
	store, err := newMetadataStore(filepath.Join(dataDir, "metadata.json"))
	if err != nil {
		return nil, err
	}
	page, err := template.ParseFiles(filepath.Join("static", "index.html"))
	if err != nil {
		return nil, err
	}
	s := &uploadServer{
		store:    store,
		chunkDir: filepath.Join(dataDir, "chunks"),
		fileDir:  filepath.Join(dataDir, "files"),
		page:     page,
	}
	if err := os.MkdirAll(s.chunkDir, 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.fileDir, 0755); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *uploadServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("POST /api/upload/init", s.handleInit)
	mux.HandleFunc("GET /api/upload/status", s.handleStatus)
	mux.HandleFunc("POST /api/upload/chunk", s.handleChunk)
	mux.HandleFunc("POST /api/upload/complete", s.handleComplete)
	mux.Handle("GET /files/", http.StripPrefix("/files/", http.FileServer(http.Dir(s.fileDir))))
	return logRequest(mux)
}

func logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

func (s *uploadServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	_ = s.page.Execute(w, map[string]any{"DefaultChunkSize": defaultChunkSize})
}

type initRequest struct {
	FileName    string `json:"fileName"`
	FileSize    int64  `json:"fileSize"`
	FileHash    string `json:"fileHash"`
	ChunkSize   int64  `json:"chunkSize"`
	TotalChunks int    `json:"totalChunks"`
}

type uploadStateResponse struct {
	Instant        bool    `json:"instant"`
	Completed      bool    `json:"completed"`
	FileURL        string  `json:"fileUrl,omitempty"`
	UploadedChunks []int   `json:"uploadedChunks"`
	Progress       float64 `json:"progress"`
	Message        string  `json:"message,omitempty"`
}

func (s *uploadServer) handleInit(w http.ResponseWriter, r *http.Request) {
	var req initRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := validateInit(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.store.mu.Lock()
	defer s.store.mu.Unlock()

	rec, ok := s.store.records[req.FileHash]
	if ok && rec.Status == "completed" && rec.Path != "" && fileExists(rec.Path) {
		writeJSON(w, http.StatusOK, uploadStateResponse{
			Instant:        true,
			Completed:      true,
			FileURL:        fileURL(rec),
			UploadedChunks: uploadedList(rec),
			Progress:       1,
			Message:        "same file already uploaded",
		})
		return
	}
	if !ok {
		rec = &uploadRecord{
			FileHash:    req.FileHash,
			FileName:    safeFileName(req.FileName),
			FileSize:    req.FileSize,
			ChunkSize:   req.ChunkSize,
			TotalChunks: req.TotalChunks,
			Uploaded:    map[int]bool{},
			Status:      "uploading",
		}
		s.store.records[req.FileHash] = rec
	} else {
		rec.FileName = safeFileName(req.FileName)
		rec.FileSize = req.FileSize
		rec.ChunkSize = req.ChunkSize
		rec.TotalChunks = req.TotalChunks
		rec.Status = "uploading"
	}
	rec.UpdatedAt = time.Now()
	if err := s.store.saveLocked(); err != nil {
		writeError(w, http.StatusInternalServerError, "save metadata failed")
		return
	}
	writeJSON(w, http.StatusOK, s.stateResponseLocked(rec))
}

func (s *uploadServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	hash := strings.TrimSpace(r.URL.Query().Get("hash"))
	if hash == "" {
		writeError(w, http.StatusBadRequest, "hash required")
		return
	}
	s.store.mu.Lock()
	defer s.store.mu.Unlock()

	rec, ok := s.store.records[hash]
	if !ok {
		writeError(w, http.StatusNotFound, "upload not found")
		return
	}
	writeJSON(w, http.StatusOK, s.stateResponseLocked(rec))
}

func (s *uploadServer) handleChunk(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	hash := r.FormValue("fileHash")
	index, err := strconv.Atoi(r.FormValue("index"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid chunk index")
		return
	}
	file, header, err := r.FormFile("chunk")
	if err != nil {
		writeError(w, http.StatusBadRequest, "chunk file required")
		return
	}
	defer file.Close()

	s.store.mu.Lock()
	defer s.store.mu.Unlock()

	rec, ok := s.store.records[hash]
	if !ok {
		writeError(w, http.StatusNotFound, "upload not initialized")
		return
	}
	if rec.Status == "completed" {
		writeJSON(w, http.StatusOK, s.stateResponseLocked(rec))
		return
	}
	if index < 0 || index >= rec.TotalChunks {
		writeError(w, http.StatusBadRequest, "chunk index out of range")
		return
	}
	if err := s.saveChunkLocked(rec, index, file, header); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rec.Uploaded[index] = true
	rec.UpdatedAt = time.Now()
	if err := s.store.saveLocked(); err != nil {
		writeError(w, http.StatusInternalServerError, "save metadata failed")
		return
	}
	writeJSON(w, http.StatusOK, s.stateResponseLocked(rec))
}

func (s *uploadServer) handleComplete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FileHash string `json:"fileHash"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	s.store.mu.Lock()
	defer s.store.mu.Unlock()

	rec, ok := s.store.records[req.FileHash]
	if !ok {
		writeError(w, http.StatusNotFound, "upload not found")
		return
	}
	if rec.Status == "completed" && rec.Path != "" && fileExists(rec.Path) {
		writeJSON(w, http.StatusOK, s.stateResponseLocked(rec))
		return
	}
	if len(rec.Uploaded) != rec.TotalChunks {
		writeError(w, http.StatusConflict, "not all chunks uploaded")
		return
	}
	finalPath, actualHash, err := s.mergeChunksLocked(rec)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if actualHash != rec.FileHash {
		_ = os.Remove(finalPath)
		writeError(w, http.StatusBadRequest, "sha256 mismatch")
		return
	}
	rec.Path = finalPath
	rec.Status = "completed"
	rec.UpdatedAt = time.Now()
	if err := s.store.saveLocked(); err != nil {
		writeError(w, http.StatusInternalServerError, "save metadata failed")
		return
	}
	_ = os.RemoveAll(filepath.Join(s.chunkDir, rec.FileHash))
	writeJSON(w, http.StatusOK, s.stateResponseLocked(rec))
}

func validateInit(req initRequest) error {
	if req.FileName == "" {
		return errors.New("fileName required")
	}
	if req.FileSize < 0 {
		return errors.New("fileSize must be non-negative")
	}
	if req.FileHash == "" || len(req.FileHash) != 64 {
		return errors.New("fileHash must be sha256 hex")
	}
	if req.ChunkSize <= 0 {
		return errors.New("chunkSize must be positive")
	}
	if req.TotalChunks <= 0 {
		return errors.New("totalChunks must be positive")
	}
	return nil
}

func (s *uploadServer) saveChunkLocked(rec *uploadRecord, index int, file multipart.File, header *multipart.FileHeader) error {
	dir := filepath.Join(s.chunkDir, rec.FileHash)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	partPath := filepath.Join(dir, fmt.Sprintf("%06d.part", index))
	tmpPath := partPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(out, file)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return closeErr
	}
	expectedMax := rec.ChunkSize
	if index == rec.TotalChunks-1 {
		remain := rec.FileSize - int64(index)*rec.ChunkSize
		if remain >= 0 {
			expectedMax = remain
		}
	}
	if written > rec.ChunkSize || (expectedMax >= 0 && written > expectedMax) {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("chunk too large: %s", header.Filename)
	}
	return os.Rename(tmpPath, partPath)
}

func (s *uploadServer) mergeChunksLocked(rec *uploadRecord) (string, string, error) {
	dir := filepath.Join(s.chunkDir, rec.FileHash)
	finalDir := filepath.Join(s.fileDir, rec.FileHash)
	if err := os.MkdirAll(finalDir, 0755); err != nil {
		return "", "", err
	}
	finalPath := filepath.Join(finalDir, rec.FileName)
	tmpPath := finalPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return "", "", err
	}
	hasher := sha256.New()
	w := io.MultiWriter(out, hasher)
	var written int64
	for i := 0; i < rec.TotalChunks; i++ {
		partPath := filepath.Join(dir, fmt.Sprintf("%06d.part", i))
		in, err := os.Open(partPath)
		if err != nil {
			_ = out.Close()
			_ = os.Remove(tmpPath)
			return "", "", fmt.Errorf("open chunk %d: %w", i, err)
		}
		n, copyErr := io.Copy(w, in)
		closeErr := in.Close()
		written += n
		if copyErr != nil {
			_ = out.Close()
			_ = os.Remove(tmpPath)
			return "", "", copyErr
		}
		if closeErr != nil {
			_ = out.Close()
			_ = os.Remove(tmpPath)
			return "", "", closeErr
		}
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", "", err
	}
	if written != rec.FileSize {
		_ = os.Remove(tmpPath)
		return "", "", fmt.Errorf("merged size mismatch: got %d want %d", written, rec.FileSize)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", "", err
	}
	return finalPath, hex.EncodeToString(hasher.Sum(nil)), nil
}

func (s *uploadServer) stateResponseLocked(rec *uploadRecord) uploadStateResponse {
	completed := rec.Status == "completed"
	progress := 0.0
	if rec.TotalChunks > 0 {
		progress = float64(len(rec.Uploaded)) / float64(rec.TotalChunks)
	}
	if completed {
		progress = 1
	}
	return uploadStateResponse{
		Instant:        completed,
		Completed:      completed,
		FileURL:        fileURL(rec),
		UploadedChunks: uploadedList(rec),
		Progress:       progress,
	}
}

func uploadedList(rec *uploadRecord) []int {
	chunks := make([]int, 0, len(rec.Uploaded))
	for i := range rec.Uploaded {
		chunks = append(chunks, i)
	}
	sort.Ints(chunks)
	return chunks
}

func fileURL(rec *uploadRecord) string {
	if rec.Status != "completed" || rec.Path == "" {
		return ""
	}
	return "/files/" + rec.FileHash + "/" + rec.FileName
}

func safeFileName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == string(filepath.Separator) || name == "" {
		return "upload.bin"
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	return replacer.Replace(name)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	dataDir := flag.String("data", "data", "upload data directory")
	flag.Parse()

	srv, err := newUploadServer(*dataDir)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("file upload service listening on http://localhost%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, srv.routes()))
}
