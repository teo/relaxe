/* === This file is part of Tomahawk Player - <http://tomahawk-player.org> ===
 *
 *   Copyright 2013, Teo Mrnjavac <teo@kde.org>
 *
 *   Tomahawk is free software: you can redistribute it and/or modify
 *   it under the terms of the GNU General Public License as published by
 *   the Free Software Foundation, either version 3 of the License, or
 *   (at your option) any later version.
 *
 *   Tomahawk is distributed in the hope that it will be useful,
 *   but WITHOUT ANY WARRANTY; without even the implied warranty of
 *   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
 *   GNU General Public License for more details.
 *
 *   You should have received a copy of the GNU General Public License
 *   along with Tomahawk. If not, see <http://www.gnu.org/licenses/>.
 */

package bundle

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"github.com/teo/relaxe/common"
	"github.com/teo/relaxe/makeaxe/util"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"
)

const (
	bundleVersion = "2"
)

func Package(inputPath string, outputPath string, release bool, force bool) (*common.Axe_v2, string, error) {
	metadataRelPath := "content/metadata.json"
	metadataPath := path.Join(inputPath, metadataRelPath)

	ex, err := util.ExistsFile(metadataPath)
	if err != nil {
		return nil, "", err
	}
	if !ex {
		return nil, "", fmt.Errorf("Cannot find metadata file in %v. Make sure %v exists and is readable.",
			inputPath, metadataRelPath)
	}

	metadataBytes, err := ioutil.ReadFile(metadataPath)
	if err != nil {
		return nil, "", err
	}

	metadata := common.Axe_v2{}
	err = json.Unmarshal(metadataBytes, &metadata)

	if err != nil || !common.Axe_v2check(&metadata) {
		return nil, "", fmt.Errorf("Bad metadata file in %v.", metadataPath)
	}
	pluginName := metadata.PluginName
	version := metadata.Version

	if metadata.Author != "" || metadata.Email != "" {
		fmt.Println("Warning: author and email fields are deprecated in metadata.json. Replace them with Authors array.")
		if len(metadata.Authors) == 0 {
			metadata.Authors = append(metadata.Authors, struct {
				Name  string `json:"name"`
				Email string `json:"email"`
			}{metadata.Author, metadata.Email})
		}
	}

	outputFileName := pluginName + "-" + version + ".axe"
	outputFilePath := path.Join(outputPath, outputFileName)

	ex, err = util.ExistsFile(outputFilePath)
	if !force && (ex || err != nil) { //if we don't force, and the target either exists or we're not sure
		fmt.Printf("* %v already exists, skipping.\n", outputFileName)
		return nil, outputFilePath, nil
	}

	// Let's add some stuff to the metadata file, this is information that's much
	// easier to fill in automatically now than manually whenever.
	//   * Timestamp of right now i.e. packaging time.
	//   * Git revision because it makes sense, especially during development.
	//   * Bundle format version, which might never be used but we add it just in
	//     case we ever need to distinguish one bundle format from another.
	metadata.Timestamp = time.Now().Unix()
	metadata.BundleVersion = bundleVersion
	if !release {
		gitCmd := exec.Command("git", "rev-parse", "--short", "HEAD")
		gitCmd.Dir = inputPath
		revision, err := gitCmd.Output()
		if err == nil { //we are in a git repo
			metadata.Revision = strings.TrimSpace(string(revision))
		} else {
			fmt.Printf("Warning: cannot get revision hash for %v-%v.\n", pluginName, version)
		}
	}

	metadataToWrite, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return nil, "", err
	}

	// Let's do some zipping according to the manifest.
	filesToZip := []string{}
	m := metadata.Manifest
	filesToZip = append(filesToZip, path.Join("content", m.Main))
	if m.Scripts != nil {
		for _, s := range m.Scripts {
			filesToZip = append(filesToZip, path.Join("content", s))
		}
	}
	filesToZip = append(filesToZip, path.Join("content", m.Icon))
	if m.Resources != nil {
		for _, s := range m.Resources {
			filesToZip = append(filesToZip, path.Join("content", s))
		}
	}

	ex, err = util.ExistsFile(outputFilePath)
	if ex || err != nil {
		if err := os.Remove(outputFilePath); err != nil {
			return nil, "", err
		}
	}

	f, err := os.Create(outputFilePath)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()

	z := zip.NewWriter(f)
	defer z.Close()
	for _, fileName := range filesToZip {
		currentFile, err := z.Create(fileName)
		if err != nil {
			return nil, "", err
		}
		body, err := ioutil.ReadFile(path.Join(inputPath, fileName))
		if err != nil {
			return nil, "", err
		}
		_, err = currentFile.Write(body)
		if err != nil {
			return nil, "", err
		}
	}
	currentFile, err := z.Create(metadataRelPath)
	if err != nil {
		return nil, "", err
	}
	_, err = currentFile.Write(metadataToWrite)
	if err != nil {
		return nil, "", err
	}

	sumFile, err := util.Md5sum(outputFilePath)
	if err != nil {
		fmt.Printf("Warning: could not create MD5 hash file for %v.\n", outputFileName)
	}
	sumFile += "\t" + outputFileName
	sumFilePath := path.Join(outputPath, pluginName+"-"+version+".md5")
	err = ioutil.WriteFile(sumFilePath, []byte(sumFile), 0644)

	return &metadata, outputFilePath, nil
}
