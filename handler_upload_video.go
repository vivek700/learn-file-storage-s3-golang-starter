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
	"os/exec"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	const uploadLimit = 1 << 30
	r.Body = http.MaxBytesReader(w, r.Body, uploadLimit)
	videoIDString := r.PathValue("videoID")

	videoID, err := uuid.Parse(videoIDString)

	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid video ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "couldn't find jwt", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "couldn't validate jwt", err)
		return
	}

	videoMetaData, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "couldn't get the video", err)
		return
	}

	if videoMetaData.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "nauthorized to update this video", err)
		return
	}

	videoFile, videoHeader, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "couln't parse the form file", err)
		return
	}

	defer videoFile.Close()

	mediaType, _, err := mime.ParseMediaType(videoHeader.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Content-Type", err)
		return
	}

	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Invalid file type", nil)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		log.Printf("error creating temp file: %s", err.Error())
		respondWithError(w, http.StatusInternalServerError, "Internal server Error", nil)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	_, err = io.Copy(tempFile, videoFile)
	if err != nil {
		log.Printf("error copying file: %s", err.Error())
		respondWithError(w, http.StatusInternalServerError, "Internal server Error", nil)
		return
	}

	_, err = tempFile.Seek(0, io.SeekStart)

	if err != nil {
		log.Printf("error resetting seek option: %s", err.Error())
		respondWithError(w, http.StatusInternalServerError, "Internal server Error", nil)
		return
	}

	aspectRatio, err := getVideoAspectRatio(tempFile.Name())

	rBytes := make([]byte, 32)
	rand.Read(rBytes)
	rString := base64.RawURLEncoding.EncodeToString(rBytes)

	prefix := ""
	if aspectRatio == "16:9" {
		prefix = "landscape"
	} else if aspectRatio == "9:16" {
		prefix = "portrait"
	} else {
		prefix = "other"
	}

	processedVideoPath, err := processVideoForFastStart(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "couldn't process the video for fast start", err)
		return
	}

	defer os.Remove(processedVideoPath)

	processedFile, err := os.Open(processedVideoPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "couldn't open the video file for reading", err)
		return
	}

	defer processedFile.Close()

	key := fmt.Sprintf("%s/%s%s", prefix, rString, ".mp4")

	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      aws.String(cfg.s3Bucket),
		Key:         aws.String(key),
		ContentType: aws.String(mediaType),
		Body:        processedFile,
	})

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "couldn't upload the video", err)
		return
	}

	videoUrl := fmt.Sprintf("https://%s/%s", cfg.s3CfDistribution, key)

	// videoUrl := fmt.Sprintf("%s,%s", cfg.s3Bucket, key)
	videoMetaData.VideoURL = aws.String(videoUrl)
	err = cfg.db.UpdateVideo(videoMetaData)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "couldn't upload the video", err)
		return
	}

	// videoMetaData, err = cfg.dbVideoToSignedVideo(videoMetaData)
	// if err != nil {
	// 	respondWithError(w, http.StatusInternalServerError, "couln't assign video url", err)
	// 	return
	// }

	respondWithJSON(w, http.StatusOK, videoMetaData)

}

func processVideoForFastStart(filePath string) (string, error) {
	outputFilePath := fmt.Sprintf("%s.processing", filePath)
	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outputFilePath)
	err := cmd.Run()
	if err != nil {
		return "", err
	}

	fileInfo, err := os.Stat(outputFilePath)
	if err != nil {
		return "", err
	}

	if fileInfo.Size() == 0 {
		return "", fmt.Errorf("processed file is empty")
	}

	return outputFilePath, nil
}

// func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {
//
// 	s3PresignedClient := s3.NewPresignClient(s3Client)
//
// 	v4PresignedHttpRequest, err := s3PresignedClient.PresignGetObject(context.TODO(), &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)}, s3.WithPresignExpires(expireTime))
//
// 	if err != nil {
// 		return "", err
// 	}
// 	return v4PresignedHttpRequest.URL, nil
// }

// func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {
//
// 	if video.VideoURL == nil {
// 		return video, nil
// 	}
// 	urlParts := strings.Split(*video.VideoURL, ",")
// 	if len(urlParts) < 2 {
// 		return video, nil
// 	}
// 	bucket, key := urlParts[0], urlParts[1]
// 	presignedUrl, err := generatePresignedURL(cfg.s3Client, bucket, key, (time.Minute * 10))
// 	if err != nil {
// 		return video, err
// 	}
//
// 	video.VideoURL = aws.String(presignedUrl)
// 	return video, nil
//
// }
