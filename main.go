package main

import (
    "io"
    "os"
    "fmt"
    "net"
    "time"
    "strconv"
    "net/http"
    "crypto/sha256"
    "io/ioutil"
    "mime/multipart"
    "encoding/base64"
    "gorm.io/gorm"
    "gorm.io/driver/sqlite"
    "github.com/gin-gonic/gin"
    "github.com/gin-gonic/contrib/static"
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

    // gerar nome indentificador do arquivo
    hash, err := get_file_hash(file)
    basename := generate_handle_name()
    filepath := save_folder + basename 
    if err != nil {
        c.String(http.StatusInternalServerError, "Internal server error")
        return
    }
    var blacklist blacklist
    db.First(&blacklist, "hash = ?", hash)
    if len(blacklist.Hash) > 0 {
        c.String(http.StatusBadRequest, "Blacklisted file")
        return
    }

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

    // banco de dados
    db, err := gorm.Open(sqlite.Open("wah.db"), &gorm.Config{})
    if err != nil { panic(err) }
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

    r.GET("/files", handle_files)

    r.Use(static.Serve("/", static.LocalFile("./views", true)))

    // iniciar servidor
    r.Run(":8080")
}

