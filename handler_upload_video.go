package main

import (
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"

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

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't GetVideo", err)
		return
	}
	if userID != video.UserID {
		respondWithError(w, http.StatusUnauthorized, "Not your video", err)
		return
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't read FormFile", err)
		return
	}
	defer file.Close()

	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Content-Type", err)
		return
	}
	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Invalid file type", nil)
		return
	}

	const tName = "tubely-upload.mp4"
	tFile, err := os.CreateTemp("", tName)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create temp file", err)
		return
	}
	defer os.Remove(tFile.Name())
	defer tFile.Close()

	if _, err = io.Copy(tFile, file); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't copy to temp file", err)
		return
	}
	if _, err = tFile.Seek(0, io.SeekStart); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't seek start of temp file", err)
		return
	}

	pFileName, err := processVideoForFastStart(tFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't process Video for fast start", err)
		return
	}
	pFile, err := os.Open(pFileName)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't open processed Video file", err)
		return
	}
	defer os.Remove(pFile.Name())
	defer pFile.Close()

	assetPath := getAssetPath(mediaType)

	ratio, err := getVideoAspectRatio(pFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get aspect ratio", err)
		return
	}
	switch ratio {
	case "16:9":
		assetPath = filepath.Join("landscape", assetPath)
	case "9:16":
		assetPath = filepath.Join("portrait", assetPath)
	default:
		assetPath = filepath.Join("other", assetPath)
	}

	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      aws.String(cfg.s3Bucket),
		Key:         aws.String(assetPath),
		Body:        pFile,
		ContentType: aws.String(mediaType),
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't upload file to s3", err)
	}

	url := cfg.getVideoURL(assetPath)
	video.VideoURL = &url

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't UpdateVideo", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
