package gitlab

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/caas-team/sparrow/pkg/checks"
	"github.com/jarcoal/httpmock"
)

func Test_gitlab_fetchFileList(t *testing.T) {
	type file struct {
		Name string `json:"name"`
	}
	tests := []struct {
		name     string
		want     []string
		wantErr  bool
		mockBody []file
		mockCode int
	}{
		{
			name:     "success - 0 targets",
			want:     nil,
			wantErr:  false,
			mockCode: http.StatusOK,
			mockBody: []file{},
		},
		{
			name: "success - 1 target",
			want: []string{
				"test",
			},
			wantErr:  false,
			mockCode: http.StatusOK,
			mockBody: []file{
				{
					Name: "test",
				},
			},
		},
		{
			name: "success - 2 targets",
			want: []string{
				"test",
				"test2",
			},
			wantErr:  false,
			mockCode: http.StatusOK,
			mockBody: []file{
				{
					Name: "test",
				},
				{
					Name: "test2",
				},
			},
		},
		{
			name:     "failure - API error",
			want:     nil,
			wantErr:  true,
			mockCode: http.StatusInternalServerError,
		},
	}

	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := httpmock.NewJsonResponder(tt.mockCode, tt.mockBody)
			if err != nil {
				t.Fatalf("error creating mock response: %v", err)
			}
			httpmock.RegisterResponder("GET", "http://test/api/v4/projects/1/repository/tree?ref=main", resp)

			g := &Client{
				baseUrl:   "http://test",
				projectID: 1,
				token:     "test",
				client:    http.DefaultClient,
			}
			got, err := g.fetchFileList(context.Background())
			if (err != nil) != tt.wantErr {
				t.Fatalf("FetchFiles() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("FetchFiles() got = %v, want %v", got, tt.want)
			}
		})
	}
}

// The filelist and url are the same, so we HTTP responders can
// be created without much hassle
func Test_gitlab_fetchFiles(t *testing.T) {
	tests := []struct {
		name     string
		want     []checks.GlobalTarget
		fileList []string
		wantErr  bool
		mockCode int
	}{
		{
			name:     "success - 0 targets",
			want:     nil,
			wantErr:  false,
			mockCode: http.StatusOK,
		},
		{
			name: "success - 1 target",
			want: []checks.GlobalTarget{
				{
					Url:      "test",
					LastSeen: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
				},
			},
			fileList: []string{
				"test",
			},
			wantErr:  false,
			mockCode: http.StatusOK,
		},
		{
			name: "success - 2 targets",
			want: []checks.GlobalTarget{
				{
					Url:      "test",
					LastSeen: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
				},
				{
					Url:      "test2",
					LastSeen: time.Date(2021, 2, 1, 0, 0, 0, 0, time.UTC),
				},
			},
			fileList: []string{
				"test",
				"test2",
			},
			wantErr:  false,
			mockCode: http.StatusOK,
		},
	}

	httpmock.Activate()
	defer httpmock.DeactivateAndReset()
	g := &Client{
		baseUrl:   "http://test",
		projectID: 1,
		token:     "test",
		client:    http.DefaultClient,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// setup mock responses
			for i, target := range tt.want {
				resp, err := httpmock.NewJsonResponder(tt.mockCode, target)
				if err != nil {
					t.Fatalf("error creating mock response: %v", err)
				}
				httpmock.RegisterResponder("GET", fmt.Sprintf("http://test/api/v4/projects/1/repository/files/%s/raw?ref=main", tt.fileList[i]), resp)
			}

			got, err := g.fetchFiles(context.Background(), tt.fileList)
			if (err != nil) != tt.wantErr {
				t.Fatalf("FetchFiles() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("FetchFiles() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_gitlab_fetchFiles_error_cases(t *testing.T) {
	type mockResponses struct {
		response checks.GlobalTarget
		err      bool
	}

	tests := []struct {
		name          string
		mockResponses []mockResponses
		fileList      []string
	}{
		{
			name: "failure - direct API error",
			mockResponses: []mockResponses{
				{
					err: true,
				},
			},
			fileList: []string{
				"test",
			},
		},
		{
			name: "failure - API error after one successful request",
			mockResponses: []mockResponses{
				{
					response: checks.GlobalTarget{
						Url:      "test",
						LastSeen: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
					},
					err: false,
				},
				{
					response: checks.GlobalTarget{},
					err:      true,
				},
			},
			fileList: []string{
				"test",
				"test2-will-fail",
			},
		},
	}

	httpmock.Activate()
	defer httpmock.DeactivateAndReset()
	g := &Client{
		baseUrl:   "http://test",
		projectID: 1,
		token:     "test",
		client:    http.DefaultClient,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i, target := range tt.mockResponses {
				if target.err {
					errResp := httpmock.NewStringResponder(http.StatusInternalServerError, "")
					httpmock.RegisterResponder("GET", fmt.Sprintf("http://test/api/v4/projects/1/repository/files/%s/raw?ref=main", tt.fileList[i]), errResp)
					continue
				}
				resp, err := httpmock.NewJsonResponder(http.StatusOK, target)
				if err != nil {
					t.Fatalf("error creating mock response: %v", err)
				}
				httpmock.RegisterResponder("GET", fmt.Sprintf("http://test/api/v4/projects/1/repository/files/%s/raw?ref=main", tt.fileList[i]), resp)
			}

			_, err := g.fetchFiles(context.Background(), tt.fileList)
			if err == nil {
				t.Fatalf("Expected error but got none.")
			}
		})
	}
}

func TestClient_PutFile(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		file     File
		mockCode int
		wantErr  bool
	}{
		{
			name: "success",
			file: File{
				Branch:      "main",
				AuthorEmail: "test@sparrow",
				AuthorName:  "sparrpw",
				Content: checks.GlobalTarget{
					Url:      "https://test.de",
					LastSeen: now,
				},
				CommitMessage: "test-commit",
				fileName:      "test.de.json",
			},
			mockCode: http.StatusOK,
		},
		{
			name: "failure - API error",
			file: File{
				Branch:      "main",
				AuthorEmail: "test@sparrow",
				AuthorName:  "sparrpw",
				Content: checks.GlobalTarget{
					Url:      "https://test.de",
					LastSeen: now,
				},
				CommitMessage: "test-commit",
				fileName:      "test.de.json",
			},
			mockCode: http.StatusInternalServerError,
			wantErr:  true,
		},
		{
			name:    "failure - empty file",
			wantErr: true,
		},
	}

	httpmock.Activate()
	defer httpmock.DeactivateAndReset()
	g := &Client{
		baseUrl:   "http://test",
		projectID: 1,
		token:     "test",
		client:    http.DefaultClient,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr {
				resp := httpmock.NewStringResponder(tt.mockCode, "")
				httpmock.RegisterResponder("PUT", fmt.Sprintf("http://test/api/v4/projects/1/repository/files/%s", tt.file.fileName), resp)
			} else {
				resp, err := httpmock.NewJsonResponder(tt.mockCode, tt.file)
				if err != nil {
					t.Fatalf("error creating mock response: %v", err)
				}
				httpmock.RegisterResponder("PUT", fmt.Sprintf("http://test/api/v4/projects/1/repository/files/%s", tt.file.fileName), resp)
			}

			if err := g.PutFile(context.Background(), tt.file); (err != nil) != tt.wantErr {
				t.Fatalf("PutFile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestClient_PostFile(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		file     File
		mockCode int
		wantErr  bool
	}{
		{
			name: "success",
			file: File{
				Branch:      "main",
				AuthorEmail: "test@sparrow",
				AuthorName:  "sparrpw",
				Content: checks.GlobalTarget{
					Url:      "https://test.de",
					LastSeen: now,
				},
				CommitMessage: "test-commit",
				fileName:      "test.de.json",
			},
			mockCode: http.StatusCreated,
		},
		{
			name: "failure - API error",
			file: File{
				Branch:      "main",
				AuthorEmail: "test@sparrow",
				AuthorName:  "sparrpw",
				Content: checks.GlobalTarget{
					Url:      "https://test.de",
					LastSeen: now,
				},
				CommitMessage: "test-commit",
				fileName:      "test.de.json",
			},
			mockCode: http.StatusInternalServerError,
			wantErr:  true,
		},
		{
			name:    "failure - empty file",
			wantErr: true,
		},
	}

	httpmock.Activate()
	defer httpmock.DeactivateAndReset()
	g := &Client{
		baseUrl:   "http://test",
		projectID: 1,
		token:     "test",
		client:    http.DefaultClient,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr {
				resp := httpmock.NewStringResponder(tt.mockCode, "")
				httpmock.RegisterResponder("POST", fmt.Sprintf("http://test/api/v4/projects/1/repository/files/%s", tt.file.fileName), resp)
			} else {
				resp, err := httpmock.NewJsonResponder(tt.mockCode, tt.file)
				if err != nil {
					t.Fatalf("error creating mock response: %v", err)
				}
				httpmock.RegisterResponder("POST", fmt.Sprintf("http://test/api/v4/projects/1/repository/files/%s", tt.file.fileName), resp)
			}

			if err := g.PostFile(context.Background(), tt.file); (err != nil) != tt.wantErr {
				t.Fatalf("PostFile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
