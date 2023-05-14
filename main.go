package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"crypto/sha256"
	"encoding/base64"
	"io/ioutil"
	"mime/multipart"
	"bytes"
	"html/template"
	"net/http"
	"strconv"
	"time"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	// "github.com/gin-gonic/contrib/static"
	"github.com/gin-gonic/gin"
)

const save_folder = "store/"
const max_size = 4e+6


func get_file_hash(file multipart.File) (string, error) {
    defer file.Seek(0, 0)

    h := sha256.New()
    if _, err := io.Copy(h, file); err != nil {
        return "", err
    }

    hashBytes := h.Sum(nil)
    return base64.StdEncoding.EncodeToString(hashBytes), nil
}

func generate_handle_name() string {
    return strconv.FormatInt(time.Now().UnixNano(), 10)
}

func handle_upload(c *gin.Context, db *gorm.DB) {

    c.Header("Access-Control-Allow-Origin",  "*")
    c.Header("Access-Control-Allow-Methods",  "POST")
    c.Header("Access-Control-Allow-Headers",  "Content-Type")

    file, _, err := c.Request.FormFile("file")
    if err != nil {
        c.String(http.StatusBadRequest, "Bad request")
        return
    }

    hash, err := get_file_hash(file); if err != nil {
        c.String(http.StatusInternalServerError, "Internal server error")
        return
    }
    basename := generate_handle_name()
    filepath := save_folder + basename 

	// verificar se arquivo não foi banido
    var blacklist blacklist
    db.First(&blacklist, "hash = ?", hash)
    if len(blacklist.Hash) > 0 {
        c.String(http.StatusBadRequest, "Blacklisted file")
        return
    }

    // salvar arquivo em disco
    out, err := os.Create(filepath); if err != nil {
        c.String(http.StatusInternalServerError, "Internal server error")
        return
    }
    defer out.Close()

	// verificar se arquivo não excede tamanho maximo
    content_length, err := strconv.Atoi(c.Request.Header.Get("Content-Length")); if err != nil {
        os.Remove(filepath)
        c.String(http.StatusInternalServerError, "Internal server error")
        return
    }
    if content_length > max_size {
        os.Remove(filepath)
        c.String(http.StatusBadRequest, "File size too big")
        return
    }
    _, err = io.CopyN(out, file, int64(content_length)); if err != nil && err != io.EOF {
        os.Remove(filepath)
        c.String(http.StatusBadRequest, "File size too big")
        return
    }

    // logar arquivo em banco de dados
    result := db.Create(&files{
		Hash: hash,
		Name: basename,
		Ip: c.RemoteIP(),
		Uploaded: time.Now(),
	})
    if result.Error != nil {
        os.Remove(filepath)
        panic(result.Error)
    }

    // retornar nome do arquivo
    c.JSON(http.StatusOK, basename)
}


func handle_download(c *gin.Context) {
    filename := c.Param("filename")
    file, err := os.Open(save_folder + filename); if err != nil {
        c.String(http.StatusInternalServerError, "arquivo inexistente?")
        return
    }
    defer file.Close()

    header := make([]byte, 512)
    if _, err := file.Read(header); err != nil {
        c.String(http.StatusInternalServerError,
			"erro ao ler arquivo. arquivo foi deletado?")
        return
    }

	content_type := http.DetectContentType(header)
	// servir paginas html como paginas de texto comum
	// isso evita que usem o serviço para hospedar sites
	if strings.Contains(content_type, "text/html") {
		content_type = "text/plain"
	}
    c.Header("Content-Type", content_type)
    c.File(save_folder + filename)
    // c.Header("Content-Disposition", "attachment; filename="+filename)
}


func list_files(c *gin.Context) {

    c.Header("Access-Control-Allow-Origin",  "*")
    c.Header("Access-Control-Allow-Methods",  "GET")
    c.Header("Access-Control-Allow-Headers",  "Content-Type")

    files, err := ioutil.ReadDir(save_folder)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
        return
    }
    var filenames []string
    for _, file := range files {
        if !file.IsDir() {
            filenames = append(filenames, file.Name())
        }
    }
    c.JSON(http.StatusOK, filenames)
}

type files struct {
    Id              int `gorm:"primaryKey;autoIncrement:true"`
    Hash, Name, Ip  string
    Uploaded        time.Time
}

type blacklist struct {
    Hash string `gorm:"primaryKey"`
    Time time.Time
}

func main() {
    r := gin.Default()

	// template do site de exemplo
    address := "localhost"
	var buffer bytes.Buffer
    template_data, err := ioutil.ReadFile("views/index.html"); if err != nil {
		panic(err)
	}
	tmpl, err := template.New("foo").Parse(string(template_data)); if err != nil {
		panic(err)
	}
	err = tmpl.Execute(&buffer, address); if err != nil {
		panic(err)
	}
	rendered_template := buffer.Bytes()

    // banco de dados
	dsn := "host=localhost user=wah password=wah dbname=wah port=5432 sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{}); if err != nil {
		panic(err)
	}
    db.AutoMigrate(&files{})
    db.AutoMigrate(&blacklist{})

    os.Mkdir("store", 0777)

    // deletar arquivos periodicamente
    go func () {
        for {
            hour_ago := time.Now().Add(-time.Hour)
            var result []files
            db.Where("uploaded < ?", hour_ago).Find(&result)
            if len(result) > 0 {
                for i := range result {
                    os.Remove(save_folder + result[i].Name)
                    fmt.Println(save_folder + result[i].Name)
                }
                db.Delete(result)
                fmt.Println(time.Now(), len(result), "file(s) deleted")
            }
            time.Sleep(time.Second * 100)
        }
    }()

    r.POST("/upload", func(c *gin.Context) {
        handle_upload(c, db)
    })
    r.GET("/download/:filename", handle_download)

    r.GET("/files", list_files)

	// index
	r.GET("/", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", rendered_template)
	})

    // iniciar servidor
    r.Run(":8080")
}



