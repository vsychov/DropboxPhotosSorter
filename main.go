package main

import (
	"runtime"
	"os"
	"path/filepath"
	"log"
	"encoding/json"
	"io/ioutil"
	"fmt"
	"errors"
	"flag"
	"strconv"
	"regexp"
	"time"
	"strings"
	"encoding/hex"
	"crypto/sha512"
	"io"
)

const CAMERA_UPLOADS_DIR_NAME string = "Camera Uploads"

var uniqueProcessedFiles map[string]int64;

func main() {
	uniqueProcessedFiles = make(map[string]int64)
	dropboxDefaultPath, err := getDropboxCameraUploadsPath()
	if err != nil {
		log.Println(err)
	}

	dropboxCameraUploadPath := flag.String("dropboxCameraUploadPath", dropboxDefaultPath, "path to processing dir")
	isNeedRenameFiles := flag.Bool("isNeedRenameFiles", true, "rename files to YYYY-mm-dd H.i.s.ext format, or leave as is (add number in file end, if name duplicate)")
	isNeedSortRecursive := flag.Bool("isNeedSortRecursive", true, "sort only specyfied dir, or all internal dirs too")
	isNeedRemoveDuplicates := flag.Bool("isNeedRemoveDuplicates", false, "if true, script will remove all file duplicates (use sha512)")

	flag.Parse();
	fmt.Println(*dropboxCameraUploadPath)
	fmt.Println(fmt.Sprintf("Need to rename: %s", strconv.FormatBool(*isNeedRenameFiles)))
	fmt.Println(fmt.Sprintf("Need to sort recursive: %s", strconv.FormatBool(*isNeedSortRecursive)))

	sortDirectory(dropboxDefaultPath, "", *isNeedRenameFiles, *isNeedSortRecursive, *isNeedRemoveDuplicates);
}

//recursive call method
func sortDirectory(rootDirectoryPath string, sortDirectoryPath string, isNeedToRename bool, isSortRecursive bool, isNeedRemoveDuplicates bool) {
	if sortDirectoryPath == "" {
		//for first call
		sortDirectoryPath = rootDirectoryPath;
	}

	filesToSort, err := filepath.Glob(filepath.Join(sortDirectoryPath, "*"))
	if err != nil {
		log.Fatal(err)
	}

	for _, fileOrDirAbsPath := range filesToSort {
		if info, err := os.Stat(fileOrDirAbsPath); err == nil && info.IsDir() {
			if isSortRecursive {
				log.Println("Process dir ", fileOrDirAbsPath)
				sortDirectory(rootDirectoryPath, fileOrDirAbsPath, isNeedToRename, isSortRecursive, isNeedRemoveDuplicates)
				os.Remove(fileOrDirAbsPath)
			} else {
				log.Println("Skip dir ", fileOrDirAbsPath)
			}
		} else {
			//sort this file, and move to root directory/date/filename
			log.Println("Process file ", fileOrDirAbsPath)
			sortFile(fileOrDirAbsPath, rootDirectoryPath, isNeedToRename, isNeedRemoveDuplicates)
		}
	}
}

func isFileNowUniqueAndWasRemoved(fileAbsPath string) bool {
	hasher := sha512.New()
	file, err := os.Open(fileAbsPath)
	if err != nil {
		log.Fatal(err)
	}

	defer file.Close()

	if _, err := io.Copy(hasher, file); err != nil {
		log.Fatal(err)
	}

	//close file, before remove
	file.Close()

	hexString := hex.EncodeToString(hasher.Sum(nil))
	if _, ok := uniqueProcessedFiles[hexString]; ok {

		err := os.Remove(fileAbsPath)
		if err == nil {
			log.Println("File ", fileAbsPath, " was removed")
		} else {
			log.Fatal(err)
		}

		return true
	}

	uniqueProcessedFiles[hexString] = 1;
	return false
}

func sortFile(fileAbsPath string, rootDirectoryPath string, isNeedToRename bool, isNeedRemoveDuplicates bool) {
	if isNeedRemoveDuplicates && isFileNowUniqueAndWasRemoved(fileAbsPath) {
		return
	}

	fileDateTime, err := detectFileDateTime(fileAbsPath)
	if err != nil {
		log.Println(err, fileAbsPath)
		return
	}

	fileExt := filepath.Ext(fileAbsPath)
	fileName := filepath.Base(fileAbsPath)
	dateFolderBase := fileDateTime.Format("2006-01")

	newDateFolderAbs := filepath.Join(rootDirectoryPath, dateFolderBase)

	if strings.ToLower(fileExt) == ".nef" {
		newDateFolderAbs = filepath.Join(rootDirectoryPath, dateFolderBase, "RAW")
	}

	videoExt := map[string]bool{
		".mov": true,
		".avi": true,
		".mp4": true,
		".mpg": true,
	}

	if _, ok := videoExt[strings.ToLower(fileExt)]; ok {
		newDateFolderAbs = filepath.Join(rootDirectoryPath, dateFolderBase, "VIDEO")
	}

	newFilePath := filepath.Join(newDateFolderAbs, fileName)
	if isNeedToRename {
		newFilePath = filepath.Join(newDateFolderAbs, fileDateTime.Format("2006-01-02 15.04.05") + fileExt)
	}

	newFilePathWithoutPrefix := newFilePath;
	if newFilePath != fileAbsPath {
		for i := 1; true; i++ {
			if _, err := os.Stat(newFilePath); err != nil {
				break;
			}

			nameWithoutExt := newFilePathWithoutPrefix[0:len(newFilePathWithoutPrefix) - len(fileExt)]
			nameWithoutExt += "_" + strconv.FormatInt(int64(i), 10);
			newFilePath = nameWithoutExt + fileExt;
			fmt.Println(newFilePath)
		}
	}

	if _, err := os.Stat(newDateFolderAbs); err != nil {
		err := os.MkdirAll(newDateFolderAbs, 0644)
		if err != nil {
			log.Panic(err)
		}
	}

	err = os.Rename(fileAbsPath, newFilePath)
	log.Println("Move ", fileAbsPath, " -> ", newFilePath)
	if err != nil {
		log.Panic(err)
	}
}

func detectFileDateTime(fileAbsPath string) (time.Time, error) {
	//first - try detect by filename, support only yyyy-MM-dd HH.mm.ss format
	//second - by last file modify time attribute

	fileNameWithoutExtRegexp := regexp.MustCompile("[.][^.]+$")

	fileNameTime, err := time.Parse("2006-01-02 15.04.05", fileNameWithoutExtRegexp.ReplaceAllString(filepath.Base(fileAbsPath), ""))
	if err == nil {
		return fileNameTime, nil
	}

	fileStat, err := os.Stat(fileAbsPath)
	if err != nil {
		return time.Now(), errors.New("unable to detect detect file time: " + fileAbsPath)
	}

	return fileStat.ModTime(), nil
}

//first - need check settings.yml file
//second, settings from dropbox config
func getDropboxCameraUploadsPath() (string, error) {
	dropboxByConfigRootPath, err := getDropboxRootPathByConfigFile()
	if (err != nil) {
		return "", err
	}

	dropboxCameraUploadPath := filepath.Join(dropboxByConfigRootPath, CAMERA_UPLOADS_DIR_NAME)
	//if personal path exists
	if _, err := os.Stat(dropboxCameraUploadPath); err == nil {
		return dropboxCameraUploadPath, nil
	}

	return "", errors.New("unable to detect camera upload path")
}

//Return path to dropbox dir, via dropbox config
//first - personal dropbox
//second - business
func getDropboxRootPathByConfigFile() (string, error) {
	dropboxConfigFilePath := getDropboxSettingsFilePath();
	dropboxConfigFileJson, err := ioutil.ReadFile(dropboxConfigFilePath)
	if err != nil {
		log.Fatal(err)
	}

	var dropboxInfo DropboxInfo
	json.Unmarshal(dropboxConfigFileJson, &dropboxInfo)

	//if personal path exists
	if _, err := os.Stat(dropboxInfo.Personal.Path); err == nil {
		return dropboxInfo.Personal.Path, nil
	}

	//if business path exists
	if _, err := os.Stat(dropboxInfo.Business.Path); err == nil {
		return dropboxInfo.Business.Path, nil
	}

	return "", errors.New("can't detect default dropbox dirrectory path")
}

//Return cross platform path to dropbox settings file
//Info - https://www.dropbox.com/help/desktop-web/find-folder-paths
func getDropboxSettingsFilePath() string {
	//dropboxCameraUploadsPath := "."
	if runtime.GOOS == "windows" {
		appDataPath := os.Getenv("APPDATA");
		localAppDataPath := os.Getenv("LOCALAPPDATA");
		dropboxAppDataJsonFilePath := filepath.Join(appDataPath, "Dropbox", "info.json");
		dropboxLocalAppDataJsonFilePath := filepath.Join(localAppDataPath, "Dropbox", "info.json");

		if _, err := os.Stat(dropboxAppDataJsonFilePath); err == nil {
			return dropboxAppDataJsonFilePath
		}

		if _, err := os.Stat(dropboxLocalAppDataJsonFilePath); err == nil {
			return dropboxLocalAppDataJsonFilePath
		}
	} else if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		dropboxJsonFilePath, err := filepath.Abs("~/.dropbox/info.json");
		if err != nil {
			log.Fatal(err)
		}

		if _, err := os.Stat(dropboxJsonFilePath); err == nil {
			return dropboxJsonFilePath
		}
	}

	return ""
}
