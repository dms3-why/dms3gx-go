package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	cli "github.com/codegangsta/cli"
	dms3gx "github.com/dms3-why/dms3gx/gxutil"
)

var pm *dms3gx.PM

var LinkCommand = cli.Command{
	Name:  "link",
	Usage: "Symlink packages to their dvcsimport repos, for local development.",
	Description: `dms3gx-go link eases local development by symlinking actual workspace repositories on demand.

Example workflow:

> dms3gx-go link QmQA5mdxru8Bh6dpC9PJfSkumqnmHgJX7knxSgBo5Lpime QmVGtdTZdTFaLsaj2RwdVG8jcjNNcp1DE914DKZ2kHmXHw QmSpJByNKFX1sCsHBEp3R73FL4NF6FnQTEGyNAXHm2GS52
linked QmQA5mdxru8Bh6dpC9PJfSkumqnmHgJX7knxSgBo5Lpime /home/user/go/src/github.com/dms3-p2p/go-p2p
linked QmVGtdTZdTFaLsaj2RwdVG8jcjNNcp1DE914DKZ2kHmXHw /home/user/go/src/github.com/dms3-mft/go-multihash
linked QmSpJByNKFX1sCsHBEp3R73FL4NF6FnQTEGyNAXHm2GS52 /home/user/go/src/github.com/dms3-fs/go-log

> dms3gx-go link
QmQA5mdxru8Bh6dpC9PJfSkumqnmHgJX7knxSgBo5Lpime /home/user/go/src/github.com/dms3-p2p/go-p2p
QmSpJByNKFX1sCsHBEp3R73FL4NF6FnQTEGyNAXHm2GS52 /home/user/go/src/github.com/dms3-fs/go-log
QmVGtdTZdTFaLsaj2RwdVG8jcjNNcp1DE914DKZ2kHmXHw /home/user/go/src/github.com/dms3-mft/go-multihash

> dms3gx-go link -r QmSpJByNKFX1sCsHBEp3R73FL4NF6FnQTEGyNAXHm2GS52
unlinked QmSpJByNKFX1sCsHBEp3R73FL4NF6FnQTEGyNAXHm2GS52 /home/user/go/src/github.com/dms3-fs/go-log

> dms3gx-go link
QmQA5mdxru8Bh6dpC9PJfSkumqnmHgJX7knxSgBo5Lpime /home/user/go/src/github.com/dms3-p2p/go-p2p
QmVGtdTZdTFaLsaj2RwdVG8jcjNNcp1DE914DKZ2kHmXHw /home/user/go/src/github.com/dms3-mft/go-multihash

> dms3gx-go link -r -a
unlinked QmQA5mdxru8Bh6dpC9PJfSkumqnmHgJX7knxSgBo5Lpime /home/user/go/src/github.com/dms3-p2p/go-p2p
unlinked QmVGtdTZdTFaLsaj2RwdVG8jcjNNcp1DE914DKZ2kHmXHw /home/user/go/src/github.com/dms3-mft/go-multihash

> dms3gx-go link
`,
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "r,remove",
			Usage: "Remove an existing symlink and reinstate the dms3gx package.",
		},
		cli.BoolFlag{
			Name:  "a,all",
			Usage: "Remove all existing symlinks and reinstate the dms3gx packages. Use with -r.",
		},
	},
	Action: func(c *cli.Context) error {
		remove := c.Bool("remove")
		all := c.Bool("all")

		hashes := c.Args()[:]
		if len(hashes) == 0 {
			links, err := listLinkedPackages()
			if err != nil {
				return err
			}

			if remove && all {
				for _, link := range links {
					hashes = append(hashes, link[0])
				}
			}

			if !remove {
				for _, link := range links {
					fmt.Printf("%s %s\n", link[0], link[1])
				}
				return nil
			}
		}
		pkg, _ := LoadPackageFile(dms3gx.PkgFileName)

		for _, hash := range hashes {
			if pkg != nil {
				// try to resolve
				if dep := pkg.FindDep(hash); dep != nil {
					hash = dep.Hash
				}
			}

			if remove {
				target, err := unlinkPackage(hash)
				if err != nil {
					return err
				}
				fmt.Printf("unlinked %s %s\n", hash, target)
			} else {
				target, err := linkPackage(hash)
				if err != nil {
					return err
				}
				fmt.Printf("linked %s %s\n", hash, target)
			}
		}

		return nil
	},
}

func listLinkedPackages() ([][]string, error) {
	var links [][]string

	srcdir, err := dms3gx.InstallPath("go", "", true)
	if err != nil {
		return links, err
	}
	dms3gxbase := filepath.Join(srcdir, "dms3gx", "dms3fs")

	filepath.Walk(dms3gxbase, func(path string, fi os.FileInfo, err error) error {
		relpath, err := filepath.Rel(dms3gxbase, path)
		if err != nil {
			return err
		}

		parts := strings.Split(relpath, string(os.PathSeparator))
		if len(parts) != 2 {
			return nil
		}

		if fi.Mode()&os.ModeSymlink != 0 {
			target, err := filepath.EvalSymlinks(path)
			if err != nil {
				return err
			}
			links = append(links, []string{parts[0], target})
		}

		return nil
	})

	return links, nil
}

// dms3gx get $hash
// go get $dvcsimport
// rm -rf $GOPATH/src/dms3gx/dms3fs/$hash/$pkgname
// ln -s $GOPATH/src/$dvcsimport $GOPATH/src/dms3gx/dms3fs/$hash/$pkgname
// cd $GOPATH/src/$dvcsimport && dms3gx install && dms3gx-go rewrite
func linkPackage(hash string) (string, error) {
	srcdir, err := dms3gx.InstallPath("go", "", true)
	if err != nil {
		return "", err
	}
	dms3gxdir := filepath.Join(srcdir, "dms3gx", "dms3fs", hash)

	dms3gxget := exec.Command("dms3gx", "get", hash, "-o", dms3gxdir)
	dms3gxget.Stdout = os.Stderr
	dms3gxget.Stderr = os.Stderr
	if err = dms3gxget.Run(); err != nil {
		return "", fmt.Errorf("error during dms3gx get: %s", err)
	}

	var pkg dms3gx.Package
	err = dms3gx.FindPackageInDir(&pkg, dms3gxdir)
	if err != nil {
		return "", fmt.Errorf("error during dms3gx.FindPackageInDir: %s", err)
	}

	dvcsimport := Dms3GxDvcsImport(&pkg)
	target := filepath.Join(srcdir, dvcsimport)
	dms3gxtarget := filepath.Join(dms3gxdir, pkg.Name)

	_, err = os.Stat(target)
	if os.IsNotExist(err) {
		goget := exec.Command("go", "get", dvcsimport+"/...")
		goget.Stdout = nil
		goget.Stderr = os.Stderr
		if err = goget.Run(); err != nil {
			return "", fmt.Errorf("error during go get: %s", err)
		}
	} else if err != nil {
		return "", fmt.Errorf("error during os.Stat: %s", err)
	}

	err = os.RemoveAll(dms3gxtarget)
	if err != nil {
		return "", fmt.Errorf("error during os.RemoveAll: %s", err)
	}

	err = os.Symlink(target, dms3gxtarget)
	if err != nil {
		return "", fmt.Errorf("error during os.Symlink: %s", err)
	}

	dms3gxinst := exec.Command("dms3gx", "install")
	dms3gxinst.Dir = target
	dms3gxinst.Stdout = nil
	dms3gxinst.Stderr = os.Stderr
	if err = dms3gxinst.Run(); err != nil {
		return "", fmt.Errorf("error during dms3gx install: %s", err)
	}

	rwcmd := exec.Command("dms3gx-go", "hook", "post-install", dms3gxdir)
	rwcmd.Dir = target
	rwcmd.Stdout = os.Stdout
	rwcmd.Stderr = os.Stderr
	if err := rwcmd.Run(); err != nil {
		return "", fmt.Errorf("error during dms3gx-go rw: %s", err)
	}

	return target, nil
}

// rm -rf $GOPATH/src/dms3gx/dms3fs/$hash
// dms3gx get $hash
func unlinkPackage(hash string) (string, error) {
	srcdir, err := dms3gx.InstallPath("go", "", true)
	if err != nil {
		return "", err
	}
	dms3gxdir := filepath.Join(srcdir, "dms3gx", "dms3fs", hash)

	err = os.RemoveAll(dms3gxdir)
	if err != nil {
		return "", fmt.Errorf("error during os.RemoveAll: %s", err)
	}

	dms3gxget := exec.Command("dms3gx", "get", hash, "-o", dms3gxdir)
	dms3gxget.Stdout = nil
	dms3gxget.Stderr = os.Stderr
	if err = dms3gxget.Run(); err != nil {
		return "", fmt.Errorf("error during dms3gx get: %s", err)
	}

	var pkg dms3gx.Package
	err = dms3gx.FindPackageInDir(&pkg, dms3gxdir)
	if err != nil {
		return "", fmt.Errorf("error during dms3gx.FindPackageInDir: %s", err)
	}

	dvcsimport := Dms3GxDvcsImport(&pkg)
	target := filepath.Join(srcdir, dvcsimport)

	uwcmd := exec.Command("dms3gx-go", "uw")
	uwcmd.Dir = target
	uwcmd.Stdout = nil
	uwcmd.Stderr = os.Stderr
	if err := uwcmd.Run(); err != nil {
		return "", fmt.Errorf("error during dms3gx-go uw: %s", err)
	}

	return target, nil
}

func Dms3GxDvcsImport(pkg *dms3gx.Package) string {
	pkgdms3gx := make(map[string]interface{})
	_ = json.Unmarshal(pkg.Dms3Gx, &pkgdms3gx)
	return pkgdms3gx["dvcsimport"].(string)
}
