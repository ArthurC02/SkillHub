package testlab

import (
	"archive/zip"
	"bytes"
	"io/fs"
	"testing"
)

func zipWithSymlink(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	header := &zip.FileHeader{Name: "link"}
	header.SetMode(fs.ModeSymlink | 0o777)
	w, err := zw.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("/etc/passwd")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestARefusedUploadSaysWhichRuleItBroke(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"executable", append([]byte{0x7f, 'E', 'L', 'F'}, bytes.Repeat([]byte{0}, 64)...),
			"不支援這種檔案類型：這是可執行檔、腳本，或 zip 以外的壓縮格式。"},
		{"pe executable", append([]byte{'M', 'Z', 0x90}, bytes.Repeat([]byte{0}, 64)...),
			"不支援這種檔案類型：這是可執行檔、腳本，或 zip 以外的壓縮格式。"},
		{"broken zip", append([]byte("PK\x03\x04"), bytes.Repeat([]byte{0xff}, 64)...),
			"不支援這種檔案類型：這個 zip 讀不出來，可能已經損毀。"},
		{"path escaping the zip", zipOf(t, map[string]string{"../escape.txt": "x"}),
			"不支援這種檔案類型：zip 裡有檔案路徑跳出壓縮檔本身（例如 ../ 或絕對路徑）。"},
		{"symlink in the zip", zipWithSymlink(t),
			"不支援這種檔案類型：zip 裡有捷徑（符號連結）。"},
		{"nested archive", zipOf(t, map[string]string{"data/inner.tar.gz": "x"}),
			"不支援這種檔案類型：zip 裡的「data/inner.tar.gz」本身又是壓縮檔；只接受一層。"},
		{"unknown binary", append([]byte{'O', 'g', 'g', 'S', 0x00}, bytes.Repeat([]byte{0x01, 0x00}, 128)...),
			"不支援這種檔案類型"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := detectContentType(bytes.NewReader(tc.data))
			if err == nil {
				t.Fatal("accepted")
			}
			if got := unsupportedTypeMessage(err); got != tc.want {
				t.Errorf("message = %q, want %q", got, tc.want)
			}
		})
	}
}
