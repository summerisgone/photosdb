package main

import (
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/rwcarlsen/goexif/exif"
	"github.com/urfave/cli/v2"
)

type PhotoInfo struct {
	FilePath    string
	MD5Hash     string
	TakenDate   time.Time
	CameraModel string
}

type DB struct {
	*sql.DB
}

func NewDB(dbPath string) (*DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	return &DB{db}, nil
}

func (db *DB) Initialize() error {
	query := `
 CREATE TABLE IF NOT EXISTS photos (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  file_path TEXT NOT NULL,
  md5_hash TEXT NOT NULL,
  taken_date DATETIME,
  camera_model TEXT,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
 );
 CREATE INDEX IF NOT EXISTS idx_md5_hash ON photos(md5_hash);
 CREATE INDEX IF NOT EXISTS idx_taken_date ON photos(taken_date);`

	_, err := db.Exec(query)
	return err
}

func (db *DB) SavePhoto(photo PhotoInfo) error {
	query := fmt.Sprintf(`INSERT INTO photos (file_path, md5_hash, taken_date, camera_model)
 VALUES ('%s', '%s', '%s', '%s')`, photo.FilePath, photo.MD5Hash, photo.TakenDate, photo.CameraModel)

	_, err := db.Exec(query, photo.FilePath, photo.MD5Hash, photo.TakenDate, photo.CameraModel)
	if err != nil {
		fmt.Printf("DB Error: %s", err)
	}
	return err
}

func (db *DB) FindByMD5(hash string) ([]PhotoInfo, error) {
	query := fmt.Sprintf(`
 SELECT file_path, md5_hash, taken_date, camera_model
 FROM photos
 WHERE md5_hash = %s
 `, hash)

	rows, err := db.Query(query, hash)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var photos []PhotoInfo
	for rows.Next() {
		var photo PhotoInfo
		err := rows.Scan(&photo.FilePath, &photo.MD5Hash, &photo.TakenDate, &photo.CameraModel)
		if err != nil {
			return nil, err
		}
		photos = append(photos, photo)
	}
	return photos, nil
}

func (db *DB) FindByDate(date time.Time) ([]PhotoInfo, error) {
	query := fmt.Sprintf(`SELECT file_path, md5_hash, taken_date, camera_model
FROM photos
WHERE DATE(taken_date) = DATE(%s)`, date)

	rows, err := db.Query(query, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var photos []PhotoInfo
	for rows.Next() {
		var photo PhotoInfo
		err := rows.Scan(&photo.FilePath, &photo.MD5Hash, &photo.TakenDate, &photo.CameraModel)
		if err != nil {
			return nil, err
		}
		photos = append(photos, photo)
	}
	return photos, nil
}

func calculateMD5(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func getExifInfo(filePath string) (time.Time, string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return time.Time{}, "", err
	}
	defer file.Close()

	x, err := exif.Decode(file)
	if err != nil {
		return time.Time{}, "", err
	}

	datetime, err := x.DateTime()
	if err != nil {
		datetime = time.Time{}
	}

	model, err := x.Get(exif.Model)
	cameraModel := ""
	if err == nil {
		cameraModel, _ = model.StringVal()
	}

	return datetime, cameraModel, nil
}

func scanPhotos(db *DB, rootPath string) error {
	return filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		ext := filepath.Ext(path)
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
			return nil
		}

		hash, err := calculateMD5(path)
		if err != nil {
			return err
		}

		takenDate, cameraModel, err := getExifInfo(path)
		if err != nil {
			log.Printf("Warning: Could not read EXIF data for %s: %v", path, err)
		}

		photo := PhotoInfo{
			FilePath:    path,
			MD5Hash:     hash,
			TakenDate:   takenDate,
			CameraModel: cameraModel,
		}

		return db.SavePhoto(photo)
	})
}

func main() {
	app := &cli.App{
		Name:  "photodb",
		Usage: "Photo library management tool",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "db",
				Value:   "photos.db",
				Usage:   "Path to SQLite database",
				EnvVars: []string{"PHOTODB_DATABASE"},
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "scan",
				Usage: "Scan photo library and save to database",
				Action: func(c *cli.Context) error {
					if c.NArg() < 1 {
						return fmt.Errorf("please provide path to photo library")
					}

					db, err := NewDB(c.String("db"))
					if err != nil {
						return err
					}
					defer db.Close()

					if err := db.Initialize(); err != nil {
						return err
					}

					return scanPhotos(db, c.Args().First())
				},
			},
			{
				Name:  "find-md5",
				Usage: "Find photo by MD5 hash",
				Action: func(c *cli.Context) error {
					if c.NArg() < 1 {
						return fmt.Errorf("please provide MD5 hash")
					}

					db, err := NewDB(c.String("db"))
					if err != nil {
						return err
					}
					defer db.Close()

					photos, err := db.FindByMD5(c.Args().First())
					if err != nil {
						return err
					}

					for _, photo := range photos {
						fmt.Printf("File: %s\nTaken: %s\nCamera: %s\n\n",
							photo.FilePath, photo.TakenDate, photo.CameraModel)
					}
					return nil
				},
			},
			{
				Name:  "find-date",
				Usage: "Find photos by date (YYYY-MM-DD)",
				Action: func(c *cli.Context) error {
					if c.NArg() < 1 {
						return fmt.Errorf("please provide date in YYYY-MM-DD format")
					}

					date, err := time.Parse("2006-01-02", c.Args().First())
					if err != nil {
						return err
					}

					db, err := NewDB(c.String("db"))
					if err != nil {
						return err
					}
					defer db.Close()

					photos, err := db.FindByDate(date)
					if err != nil {
						return err
					}

					for _, photo := range photos {
						fmt.Printf("File: %s\nMD5: %s\nTaken: %s\nCamera: %s\n\n",
							photo.FilePath, photo.MD5Hash, photo.TakenDate, photo.CameraModel)
					}
					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
