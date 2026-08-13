package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

const maxMemory = 10 << 20

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	// TODO: implement the upload here
	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "couldn't parse", err)
		return
	}

	fileData, fileHeader, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "couln't read form", err)
		return
	}

	defer fileData.Close()
	mediaType, _, err := mime.ParseMediaType(fileHeader.Header.Get("Content-Type"))

	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Content-Type", err)
		return
	}

	if mediaType != "image/jpeg" && mediaType != "image/png" {
		respondWithError(w, http.StatusBadRequest, "Invalid file type", nil)
		return
	}

	// imageData, err := io.ReadAll(fileData)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "couldn't read the image data", err)
		return
	}

	videoMetaData, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "couldn't get the video", err)
		return
	}

	if videoMetaData.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "not allowed to update this video", nil)
		return
	}

	fileExtension := strings.Split(mediaType, "/")[1]

	rb := make([]byte, 32)
	_, err = rand.Read(rb)
	if err != nil {
		log.Printf("error in creating random bytes: %s", err.Error())
		return
	}
	rString := base64.RawURLEncoding.EncodeToString(rb)

	filePath := filepath.Join(cfg.assetsRoot, fmt.Sprintf("%s.%s", rString, fileExtension))

	file, err := os.Create(filePath)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "couldn't create file", err)
		return
	}
	defer file.Close()
	_, err = io.Copy(file, fileData)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "couln't write the file on the disk", err)
	}

	// imageString := base64.StdEncoding.EncodeToString(imageData)

	thumbnailUrl := fmt.Sprintf("http://localhost:%s/assets/%s.%s", cfg.port, rString, fileExtension)

	videoMetaData.ThumbnailURL = &thumbnailUrl

	err = cfg.db.UpdateVideo(videoMetaData)
	if err != nil {
		delete(videoThumbnails, videoID)
		respondWithError(w, http.StatusInternalServerError, "couln't upload video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, videoMetaData)
}
