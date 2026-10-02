package archive

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Pack writes a gzipped tar of root. .git directories and built packages are skipped.
func Pack(root string, w io.Writer) error {
	return PackFilter(root, w, nil)
}

// PackFilter is Pack with an extra skip rule. Skip receives the path relative to root.
func PackFilter(root string, w io.Writer, skip func(rel string, isDir bool) bool) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if skip != nil && skip(rel, info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		base := filepath.Base(path)
		if base == ".git" {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.Contains(base, ".pkg.tar.") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(tw, f)
			f.Close()
			if copyErr != nil {
				return copyErr
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

// PackPaths writes a gzipped tar of the named paths under root. Absolute symlinks are left out.
func PackPaths(root string, rels []string, w io.Writer) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	for _, rel := range rels {
		abs := filepath.Join(root, rel)
		err := filepath.Walk(abs, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			name, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				link, err := os.Readlink(path)
				if err != nil {
					return err
				}
				if filepath.IsAbs(link) {
					return nil
				}
				hdr, err := tar.FileInfoHeader(info, link)
				if err != nil {
					return err
				}
				hdr.Name = filepath.ToSlash(name)
				return tw.WriteHeader(hdr)
			}
			hdr, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			hdr.Name = filepath.ToSlash(name)
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			if info.Mode().IsRegular() {
				f, err := os.Open(path)
				if err != nil {
					return err
				}
				_, copyErr := io.Copy(tw, f)
				f.Close()
				if copyErr != nil {
					return copyErr
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

// Unpack extracts a gzipped tar into dest. Paths that escape dest are rejected.
func Unpack(r io.Reader, dest string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		clean := filepath.Clean(hdr.Name)
		if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) || filepath.IsAbs(clean) {
			return fmt.Errorf("refusing path %q", hdr.Name)
		}
		target := filepath.Join(dest, clean)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode)&0o777)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
	}
}
