package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
)

func (cfg apiConfig) ensureAssetsDir() error {
	if _, err := os.Stat(cfg.assetsRoot); os.IsNotExist(err) {
		return os.Mkdir(cfg.assetsRoot, 0755)
	}
	return nil
}

func getVideoAspectRatio(filePath string) (string, error) {

	buf := bytes.NewBuffer([]byte{})
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	cmd.Stdout = buf

	err := cmd.Run()
	if err != nil {
		return "", nil
	}

	type videoStruct struct {
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
	}

	video := videoStruct{}
	err = json.Unmarshal(buf.Bytes(), &video)
	if err != nil {
		return "", err
	}

	ar := float64(video.Streams[0].Width) / float64(video.Streams[0].Height)

	if ar >= 1.7 && ar <= 1.8 {
		return "16:9", nil
	}

	if ar >= 0.5 && ar <= 0.6 {
		return "9:16", nil
	}

	return "other", nil

}
