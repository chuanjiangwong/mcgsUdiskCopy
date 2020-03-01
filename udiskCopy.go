package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"github.com/cheggaaa/pb/v3"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	help bool

	flagSrc        string
	flagDstDirName string
)

func usage() {
	flag.PrintDefaults()
}

func init() {
	flag.StringVar(&flagSrc, "i", "tpcbackup", "`Source dir` path what you want copy")
	flag.StringVar(&flagDstDirName, "o", "tpcbackup", "Dst dir name")

	flag.BoolVar(&help, "h", false, "This help")

	flag.Usage = usage
	flag.Parse()

	if help {
		flag.Usage()
		os.Exit(0)
	}

	abs, err := filepath.Abs(flagSrc)
	if err != nil {
		log.Println(err)
		flag.Usage()
		os.Exit(2)
	}
	flagSrc = abs
}

type procMounts struct {
	dev         string
	mountPoint  string
	mountFsType string
	mountOption string
}

type udisk struct {
	mount    procMounts
	progress *pb.ProgressBar

	index      int
	total      int
	dstDirName string
	srcDirMode os.FileMode
}

func (udisk *udisk) copyFile(srcFile string, dstFile string) error {
	src, err := os.Open(srcFile)
	if err != nil {
		return err
	}
	defer src.Close()

	stat, _ := src.Stat()
	mode := stat.Mode()

	dst, err := os.OpenFile(dstFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer dst.Close()

	io.Copy(dst, src)
	dst.Sync()
	return nil
}

func (udisk *udisk) Remove(old string) error {
	udiskOld := filepath.Join(udisk.mount.mountPoint, old)
	err := os.RemoveAll(udiskOld)
	if err != nil {
		log.Println(err)
	}
	return err
}

func (udisk *udisk) CopyDir(src string) error {
	dstDirName := filepath.Join(udisk.mount.mountPoint, udisk.dstDirName)
	err := os.MkdirAll(dstDirName, udisk.srcDirMode)
	if err != nil {
		log.Println(dstDirName, err)
		return err
	}

	tmpl := `{{string . "name" | green}} {{counters . | red}} {{bar . "[" "-" (cycle . ".") "." "]"}} {{percent .}}`
	udisk.progress = pb.ProgressBarTemplate(tmpl).Start64(int64(udisk.total))
	udisk.progress.Set("name", fmt.Sprintf("%d:%s", udisk.index, dstDirName))

	if strings.TrimSpace(src) == strings.TrimSpace(dstDirName) {
		return errors.New("src not invail")
	}

	err = filepath.Walk(src, func(path string, f os.FileInfo, err error) error {
		if f == nil {
			log.Println(path, err)
			return err
		}

		if path == src {
			return nil
		}

		dstNewPath := strings.Replace(path, src, dstDirName, -1)
		if !f.IsDir() {
			err = udisk.copyFile(path, dstNewPath)
		} else {
			err = os.MkdirAll(dstNewPath, f.Mode())
		}

		if err != nil {
			log.Println(path, err)
		}

		udisk.progress.Add(1)
		return err
	})

	udisk.progress.Finish()
	return err
}

func (udisk *udisk) ParseProcMountsFileLine(line string) {
	mount := strings.Fields(line)
	if len(mount) < 4 {
		return
	}

	udisk.mount.dev = mount[0]
	udisk.mount.mountPoint = mount[1]
	udisk.mount.mountFsType = mount[2]
	udisk.mount.mountOption = mount[3]
}

func ParseProcMountsFile(fstype string) ([]udisk, error) {
	udisks := make([]udisk, 0)

	fd, err := os.OpenFile("/proc/mounts", os.O_RDONLY, os.ModePerm)
	if err != nil {
		return udisks, err
	}
	defer fd.Close()

	reader := bufio.NewReader(fd)
	for {
		line, err := reader.ReadString('\n')
		if err != nil || err == io.EOF {
			break
		}

		udisk := new(udisk)
		udisk.ParseProcMountsFileLine(line)
		if udisk.mount.mountFsType == fstype {
			udisks = append(udisks, *udisk)
		}
	}

	return udisks, nil
}

func main() {
	stat, err := os.Stat(flagSrc)
	if err != nil {
		log.Fatal(err)
	}

	udisks, err := ParseProcMountsFile("vfat")
	if err != nil {
		log.Fatal(err)
	}

	var fileNumbers int
	filepath.Walk(flagSrc, func(path string, f os.FileInfo, err error) error {
		if f != nil && path != flagSrc {
			fileNumbers++
		}
		return err
	})
	log.Println("Src dir:", flagSrc)
	log.Println("Src file total numbers: ", fileNumbers)
	log.Println("Udisk dst dir:", flagDstDirName)
	log.Println("Udisk haved mount numbers: ", len(udisks))

	waiter := sync.WaitGroup{}

	for i, udisk := range udisks {
		waiter.Add(1)
		udisk := udisk
		udisk.index = i
		udisk.total = fileNumbers
		udisk.dstDirName = flagDstDirName
		udisk.srcDirMode = stat.Mode()
		go func() {
			udisk.Remove(udisk.dstDirName)
			udisk.CopyDir(flagSrc)
			waiter.Done()
		}()
	}

	waiter.Wait()
}
