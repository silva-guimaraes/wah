package main

import (
    "crypto/sha256"
    "encoding/base64"
    "fmt"
    "io"
    "io/ioutil"
    "mime/multipart"
    "net"
    "net/http"
    "os"
    "strconv"
    "time"
    "github.com/gin-gonic/gin"
    "gorm.io/driver/sqlite"
    "gorm.io/gorm"
)

const save_folder = "store/"
const max_size = 4e+6


func remove_char(s string, c byte) string {
    result := ""
    for i := 0; i < len(s); i++ {
        if s[i] != c {
            result += string(s[i])
        }
    }
    return result
}


func generate_file_hash(file multipart.File) (string, string, error) {
    defer file.Seek(0, 0)
    h := sha256.New()
    if _, err := io.Copy(h, file); err != nil {
        return "", "", err
    }

    hashBytes := h.Sum(nil)
    hash_string := base64.StdEncoding.EncodeToString(hashBytes)
    unique_name := remove_char(hash_string[:len(hash_string) / 2], '/')

    return hash_string, unique_name, nil
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

    // gerar nome indentificador do arquivo
    hash, basename, err := generate_file_hash(file)
    if err != nil {
        c.String(http.StatusInternalServerError, "Internal server error")
        return
    }
    filepath := save_folder + basename 

    // salvar arquivo em disco
    out, err := os.Create(filepath)
    if err != nil {
        c.String(http.StatusInternalServerError, "Internal server error")
        return
    }
    defer out.Close()

    content_length, err := strconv.Atoi(c.Request.Header.Get("Content-Length"))
    fmt.Println(content_length)
    if err != nil { 
        c.String(http.StatusInternalServerError, "Internal server error")
        return
    }
    if content_length > max_size {
        os.Remove(filepath)
        c.String(http.StatusBadRequest, "File size too big")
        return
    }

    _, err = io.CopyN(out, file, int64(content_length))

    if err != nil && err != io.EOF {
        os.Remove(filepath)
        c.String(http.StatusBadRequest, "File size too big")
        return
    }

    // logar arquivo em banco de dados
    ip, _, err := net.SplitHostPort(c.Request.RemoteAddr)
    if err != nil { 
        panic(err) 
    }
    result := db.Create(&files{Hash: hash, Name: basename, Ip: ip, Uploaded: time.Now()})
    if result.Error != nil {
        panic(result.Error)
    }

    // retornar nome do arquivo
    c.JSON(http.StatusOK, basename)
}


func handle_download(c *gin.Context) {
    filename := c.Param("filename")
    file, err := os.Open(save_folder + filename)
    if err != nil {
        c.String(http.StatusInternalServerError, "Internal server error")
        return
    }
    defer file.Close()
    // stat, err := file.Stat()
    if err != nil {
        c.String(http.StatusInternalServerError, "Internal server error")
        return
    }
    header := make([]byte, 512)
    if _, err := file.Read(header); err != nil {
        c.String(http.StatusInternalServerError, "Internal server error")
        return
    }
    contentType := http.DetectContentType(header)
    c.Header("Content-Type", contentType)
    c.File(save_folder + filename)
    // c.Header("Content-Disposition", "attachment; filename="+filename)
}


func handle_files(c *gin.Context) {
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
    c.Header("Access-Control-Allow-Origin",  "*")
    c.Header("Access-Control-Allow-Methods",  "GET")
    c.Header("Access-Control-Allow-Headers",  "Content-Type")
    c.JSON(http.StatusOK, filenames)
}

type files struct {
    Hash        string `gorm:"primaryKey"`
    Name, Ip    string
    Uploaded    time.Time
}

func main() {
    r := gin.Default()

    // banco de dados
    db, err := gorm.Open(sqlite.Open("test.db"), &gorm.Config{})
    if err != nil { panic(err) }
    db.AutoMigrate(&files{})
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
                fmt.Println(time.Now(), len(result), "files deleted")
            }
            time.Sleep(time.Second * 100)
        }
    }()

    r.POST("/upload", func(c *gin.Context) {
        handle_upload(c, db)
    })
    r.GET("/download/:filename", handle_download)

    r.GET("/files", handle_files)

    // iniciar servidor
    r.Run(":8080")
}

