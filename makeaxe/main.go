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

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/nu7hatch/gouuid"
	"github.com/teo/relaxe/common"
	"github.com/teo/relaxe/makeaxe/bundle"
	"github.com/teo/relaxe/makeaxe/util"
	"io/ioutil"
	"labix.org/v2/mgo"
	"os"
	"path"
	"path/filepath"
	"regexp"
)

const (
	programName        = "makeaxe"
	programDescription = "the Tomahawk resolver bundle creator"
	programVersion     = "0.1"
)

var (
	all     bool
	release bool
	force   bool
	help    bool
	ver     bool
	relaxe  bool
)

func usage() {
	fmt.Printf("*** %v - %v ***\n\n", programName, programDescription)
	fmt.Println("Usage: ./makeaxe [OPTIONS] SOURCE [DESTINATION|CONFIG]")
	fmt.Println("OPTIONS")
	flag.VisitAll(func(f *flag.Flag) {
		if len(f.Name) < 2 {
			return
		}
		fmt.Printf("\t%v\n", f.Usage)
	})
	fmt.Println("ARGUMENTS")
	fmt.Println("\tSOURCE\t\tMandatory, the path of the unpackaged base directory. " +
		"\n\t\t\tWhen building a single axe, this should be the parent of the directory that contains a metadata.json file. " +
		"\n\t\t\tIf building all resolvers (--all, -a) this should be the parent directory of all the resolvers.")

	fmt.Println("\tDESTINATION\tOptional, the path of the directory where newly built bundles (axes) should be placed. " +
		"\n\t\t\tIf unset, it is the same as the source directory. Not used when publishing to Relaxe (--relaxe, -x).")

	fmt.Println("\tCONFIG\t\tOnly when publishing to Relaxe (--relaxe, -x), the path of the Relaxe configuration file.")
}

func version() {
	fmt.Printf("%v, version %v\n", programName, programVersion)
}

func die(message string) {
	fmt.Println(message)
	fmt.Println("See ./makeaxe --help for usage information.")
	os.Exit(2)
}

func init() {
	const (
		flagAllUsage     = "--all, -a\tbuild all the resolvers in the SOURCE path's subdirectories"
		flagReleaseUsage = "--release, -r\tskip trying to add the git revision hash to a bundle"
		flagForceUsage   = "--force, -f\tbuild a bundle and overwrite even if the destination directory already contains a bundle of the same name and version"
		flagHelpUsage    = "--help, -h\tthis help message"
		flagVersionUsage = "--version, -v\tshow version information"
		flagRelaxeUsage  = "--relaxe, -x\tpublish resolvers on a Relaxe instance with the given config file, implies --release and ignores --force and DESTINATION"
	)
	flag.BoolVar(&all, "all", false, flagAllUsage)
	flag.BoolVar(&all, "a", false, flagAllUsage+" (shorthand)")
	flag.BoolVar(&release, "release", false, flagReleaseUsage)
	flag.BoolVar(&release, "r", false, flagReleaseUsage+" (shorthand)")
	flag.BoolVar(&force, "force", false, flagForceUsage)
	flag.BoolVar(&force, "f", false, flagForceUsage)
	flag.BoolVar(&help, "help", false, flagHelpUsage)
	flag.BoolVar(&help, "h", false, flagHelpUsage)
	flag.BoolVar(&ver, "version", false, flagVersionUsage)
	flag.BoolVar(&ver, "v", false, flagVersionUsage)
	flag.BoolVar(&relaxe, "relaxe", false, flagRelaxeUsage)
	flag.BoolVar(&relaxe, "x", false, flagRelaxeUsage)
}

func preparePaths(inputPath string) []string {
	inputList := []string{}
	if all {
		contents, err := ioutil.ReadDir(inputPath)
		if err != nil {
			die(err.Error())
		}
		for _, entry := range contents {
			if !entry.IsDir() {
				continue
			}
			realInputPath := path.Join(inputPath, entry.Name())
			metadataPath := path.Join(realInputPath, "content", "metadata.json")
			ex, err := util.ExistsFile(metadataPath)
			if !ex || err != nil {
				fmt.Printf("%v does not seem to be an axe directory, skipping.\n", entry.Name())
				continue
			}
			inputList = append(inputList, realInputPath)
		}

	} else {
		inputList = append(inputList, inputPath)
	}
	return inputList
}

func buildToRelaxe(inputList []string, relaxeConfig common.RelaxeConfig) {
	if !relaxe {
		die("Error: cannot push to Relaxe in directory mode.")
	}

	// Try to connect to the MongoDB instance first, bail out if we can't
	session, err := mgo.Dial(relaxeConfig.Database.ConnectionString)
	if err != nil {
		die("Error: cannot connect to Relaxe database. Reason: " + err.Error())
	}
	c := session.DB("relaxe").C("axes")

	fmt.Println("Woohoo, mgo collection:" + c.FullName)

	outputPath := relaxeConfig.CacheDirectory
	for _, inputDirPath := range inputList {
		metadata, outputFilePath, err := bundle.Package(inputDirPath, outputPath, true /*release*/, false /*force*/)
		if err != nil {
			fmt.Printf("Warning: could not build axe for directory %v.\n", path.Base(inputDirPath))
			continue
		}
		fmt.Printf("* Created axe in %v.\n", outputFilePath)

		u, err := uuid.NewV4()
		axeUuid := u.String()

		newOutputFilePath := path.Join(path.Dir(outputFilePath), metadata.PluginName+"-"+axeUuid+".axe")
		rx := regexp.MustCompile(`\.axe$`)
		outputMd5Path := rx.ReplaceAllString(outputFilePath, ".md5")
		newOutputMd5Path := path.Join(path.Dir(outputMd5Path), metadata.PluginName+"-"+axeUuid+".md5")

		fmt.Printf("About to rename:\n%v\t%v\n%v\t%v", outputFilePath, newOutputFilePath, outputMd5Path, newOutputMd5Path)

		err = os.Rename(outputFilePath, newOutputFilePath)
		if err != nil {
			fmt.Printf("Warning: could not rename axe %v. Deleting axe and md5.\n", outputFilePath)
			if err := os.Remove(outputFilePath); err != nil {
				fmt.Printf("Warning: could not rename nor delete temporary axe at %v", outputFilePath)
			}
			if err := os.Remove(outputMd5Path); err != nil {
				fmt.Printf("Warning: could not rename nor delete temporary md5 at %v", outputMd5Path)
			}
			continue
		}

		err = os.Rename(outputMd5Path, newOutputMd5Path)
		if err != nil {
			fmt.Printf("Warning: could not rename md5 %v. Deleting axe and md5.\n", outputMd5Path)
			if err := os.Remove(outputMd5Path); err != nil {
				fmt.Printf("Warning: could not rename nor delete temporary md5 at %v", outputMd5Path)
			}
			if err := os.Remove(newOutputFilePath); err != nil {
				fmt.Printf("Warning: could not rename nor delete axe at %v", newOutputFilePath)
			}
			continue
		}

		metadata.AxeId = axeUuid

		mrshld, _ := json.MarshalIndent(metadata, "", "  ")
		fmt.Println("Pushing to Relaxe:\n" + string(mrshld))
		c.Insert()
	}
}

func buildToDirectory(inputList []string, outputPath string) {
	if relaxe {
		die("Error: cannot build to directory in Relaxe mode.")
	}

	for _, inputDirPath := range inputList {
		_, outputFilePath, err := bundle.Package(inputDirPath, outputPath, release, force)
		if err != nil {
			fmt.Printf("Warning: could not build axe for directory %v.\n", path.Base(inputDirPath))
			continue
		}
		fmt.Printf("* Created axe in %v.\n", outputFilePath)
	}
}

func main() {
	flag.Parse()

	if help {
		usage()
		return
	}

	if ver {
		version()
		return
	}

	if len(flag.Args()) == 0 {
		die("Error: a source directory must be specified.")
	}

	if len(flag.Args()) > 2 {
		die("Error: too many arguments.")
	}

	// Prepare input directory path(s)
	inputPath, err := filepath.Abs(flag.Arg(0))
	if err != nil {
		die("Error: bad source directory path.")
	}
	if ex, err := util.ExistsDir(inputPath); !ex || err != nil {
		die("Error: bad source directory path.")
	}

	inputList := preparePaths(inputPath)

	// Prepare output directory path and build
	if relaxe {
		if len(flag.Args()) != 2 {
			die("Error: source or Relaxe configuration file path missing.")
		}
		configFilePath, err := filepath.Abs(flag.Arg(1))
		if err != nil {
			die("Error: bad Relaxe configuration file path.")
		}
		if ex, err := util.ExistsFile(configFilePath); !ex || err != nil {
			die("Error: bad Relaxe configuration file path.")
		}

		config, err := common.LoadConfig(configFilePath)
		if err != nil {
			die(err.Error())
		}

		buildToRelaxe(inputList, *config)

	} else {
		var outputPath string

		if len(flag.Args()) == 1 {
			outputPath = inputPath
		} else { //len is 2
			outputPath, err = filepath.Abs(flag.Arg(1))
			if err != nil {
				die("Error: bad destination directory path.")
			}
			if ex, err := util.ExistsDir(outputPath); !ex || err != nil {
				die("Error: bad destination directory path.")
			}
		}

		buildToDirectory(inputList, outputPath)
	}
}
