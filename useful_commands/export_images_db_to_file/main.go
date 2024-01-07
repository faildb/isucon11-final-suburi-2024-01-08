package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	// mysql
	_ "github.com/go-sql-driver/mysql"
	// postgres
	//_ "github.com/mackee/pgx-replaced"
)

type Post struct {
	ID      int    `db:"id"`
	Mime    string `db:"mime"`
	ImgData []byte `db:"imgdata"`
}

func main() {
	log.SetFlags(log.Lshortfile | log.LstdFlags | log.Ltime)

	exportDir := flag.String("export-dir", "/tmp/", "export directory")
	flag.Parse()

	db, err := GetDBMysql()
	if err != nil {
		log.Fatal(err)
	}

	// Retrieve image data from the database
	var posts []Post
	query := `SELECT id, mime, imgdata FROM posts`
	err = db.Select(&posts, query)
	if err != nil {
		log.Fatal(err)
	}

	// Save each image to a file
	for _, post := range posts {
		filename := filepath.Join(*exportDir, fmt.Sprintf("%d.%s", post.ID, mimeToExt(post.Mime)))
		err := os.WriteFile(filename, post.ImgData, 0666)
		if err != nil {
			log.Printf("Failed to write file for post ID %d: %v\n", post.ID, err)
			continue
		}
		fmt.Printf("File saved: %s\n", filename)
	}
}

// mimeToExt converts a MIME type to a file extension
func mimeToExt(mime string) string {
	switch mime {
	case "image/jpeg":
		return "jpg"
	case "image/png":
		return "png"
	case "image/gif":
		return "gif"
	// Add more MIME types as needed
	default:
		return "bin"
	}
}
