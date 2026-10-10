package views

import (
	_ "embed"
	"encoding/json"
	"sync"
)

// Posts are the static editorial posts powering the blogs section of the
// public homepage (reference App\Posts + resources/posts.json). The blogs
// index/show pages land with the posts wave.
//
//go:embed posts.json
var postsJSON []byte

// Post is one entry of posts.json (only the fields the homepage renders).
type Post struct {
	Slug     string `json:"slug"`
	Title    string `json:"title"`
	Category string `json:"category"`
	Date     string `json:"date"`
	Excerpt  string `json:"excerpt"`
	Initial  string `json:"initial"`
	Gradient string `json:"gradient"`
	Image    string `json:"image"`
}

var (
	postsOnce sync.Once
	postsList []Post
)

// Posts returns the embedded posts in file order.
func Posts() []Post {
	postsOnce.Do(func() {
		_ = json.Unmarshal(postsJSON, &postsList)
	})
	return postsList
}
