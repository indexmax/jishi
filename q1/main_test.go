package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestChunkUploadComplete(t *testing.T) {
	srv := newTestServer(t)
	content := []byte("hello chunked upload")
	hash := sha256Hex(content)

	initResp := postInit(t, srv, initRequest{
		FileName:    "hello.txt",
		FileSize:    int64(len(content)),
		FileHash:    hash,
		ChunkSize:   5,
		TotalChunks: 4,
	})
	if initResp.Completed || initResp.Instant {
		t.Fatalf("new upload should not be completed: %+v", initResp)
	}

	uploadChunk(t, srv, hash, 0, content[0:5])
	uploadChunk(t, srv, hash, 1, content[5:10])
	uploadChunk(t, srv, hash, 2, content[10:15])
	uploadChunk(t, srv, hash, 3, content[15:])

	done := postComplete(t, srv, hash)
	if !done.Completed {
		t.Fatalf("upload should be completed: %+v", done)
	}

	got, err := os.ReadFile(filepath.Join(srv.fileDir, hash, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("merged file mismatch: got %q want %q", got, content)
	}
}

func TestResumeAndInstantUpload(t *testing.T) {
	srv := newTestServer(t)
	content := []byte("resume upload content")
	hash := sha256Hex(content)

	postInit(t, srv, initRequest{
		FileName:    "resume.bin",
		FileSize:    int64(len(content)),
		FileHash:    hash,
		ChunkSize:   10,
		TotalChunks: 3,
	})
	uploadChunk(t, srv, hash, 0, content[:10])

	resume := postInit(t, srv, initRequest{
		FileName:    "resume.bin",
		FileSize:    int64(len(content)),
		FileHash:    hash,
		ChunkSize:   10,
		TotalChunks: 3,
	})
	if len(resume.UploadedChunks) != 1 || resume.UploadedChunks[0] != 0 {
		t.Fatalf("resume chunks mismatch: %+v", resume.UploadedChunks)
	}

	uploadChunk(t, srv, hash, 1, content[10:20])
	uploadChunk(t, srv, hash, 2, content[20:])
	postComplete(t, srv, hash)

	instant := postInit(t, srv, initRequest{
		FileName:    "resume.bin",
		FileSize:    int64(len(content)),
		FileHash:    hash,
		ChunkSize:   10,
		TotalChunks: 3,
	})
	if !instant.Instant || !instant.Completed {
		t.Fatalf("same file should be instant upload: %+v", instant)
	}
	if instant.FileURL == "" {
		t.Fatal("instant upload should include file url")
	}
}

func newTestServer(t *testing.T) *uploadServer {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	srv, err := newUploadServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	return srv
}

func postInit(t *testing.T, srv *uploadServer, req initRequest) uploadStateResponse {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/upload/init", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	srv.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("init status=%d body=%s", response.Code, response.Body.String())
	}
	var out uploadStateResponse
	if err := json.Unmarshal(response.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func uploadChunk(t *testing.T, srv *uploadServer, hash string, index int, content []byte) uploadStateResponse {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("fileHash", hash)
	_ = writer.WriteField("index", strconv.Itoa(index))
	part, err := writer.CreateFormFile("chunk", "chunk.part")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/upload/chunk", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	srv.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("chunk status=%d body=%s", response.Code, response.Body.String())
	}
	var out uploadStateResponse
	if err := json.Unmarshal(response.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func postComplete(t *testing.T, srv *uploadServer, hash string) uploadStateResponse {
	t.Helper()
	body, err := json.Marshal(map[string]string{"fileHash": hash})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/upload/complete", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	srv.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("complete status=%d body=%s", response.Code, response.Body.String())
	}
	var out uploadStateResponse
	if err := json.Unmarshal(response.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
