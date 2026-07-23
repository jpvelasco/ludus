package ec2fleet

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
	"github.com/jpvelasco/ludus/internal/config"
)

func TestResolveServerBinaryPath(t *testing.T) {
	tests := []struct {
		name, target, project, arch string
		files                       []string
		want                        string
	}{
		{"Shipping_Test Build", "ServerTarget", "TestBuild", "", []string{"ServerTarget-Linux-Test.target", "ServerTarget-Linux-Test"}, "./TestBuild/Binaries/Linux/ServerTarget-Linux-Test"},
		{"Multi-Dot Skip", "ServerTarget", "TestBuild", "", []string{"ServerTarget-Linux-Test.config.json", "ServerTarget-Linux-Test"}, "./TestBuild/Binaries/Linux/ServerTarget-Linux-Test"},
		{"Unconventional Suffix", "ServerTarget", "TestBuild", "", []string{"ServerTarget-Linux-DebugData"}, "./TestBuild/Binaries/Linux/ServerTarget-Linux-DebugData"},
		{"Dev Path Fallback", "LyraServer", "DevBuild", "arm64", []string{}, "./DevBuild/Binaries/LinuxArm64/LyraServer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Deployer{opts: DeployOptions{ServerTarget: tt.target, ProjectName: tt.project, Arch: tt.arch}}

			buildDir := t.TempDir()
			binDir := filepath.Join(buildDir, d.opts.packagedDirName(), "Binaries", config.BinariesPlatformDir(d.opts.Arch))
			if err := os.MkdirAll(binDir, 0755); err != nil {
				t.Fatalf("failed to create test directory structure: %v", err)
			}
			for _, filename := range tt.files {
				filePath := filepath.Join(binDir, filename)
				if err := os.WriteFile(filePath, []byte(""), 0644); err != nil {
					t.Fatalf("failed to create mock file %s: %v", filename, err)
				}
			}

			got := d.resolveServerBinaryPath(buildDir, d.opts.Arch)
			if got != tt.want {
				t.Errorf("want %s, got %v", tt.want, got)
			}
		})
	}
}

// helper function to create temp file structure used in create zip
func createTmpFilesys(t *testing.T, dir string, mockFiles []string) (string, string, string) {
	t.Helper()
	zPath := filepath.Join(dir, "test_archive.zip") // create zip/binary/server build paths
	wBin := filepath.Join(dir, "wrapper-binary")
	bDir := filepath.Join(dir, "build_contents")

	if err := os.WriteFile(wBin, []byte("binary-data"), 0755); err != nil {
		t.Fatalf("failed creating wrapper binary: %v", err)
	}
	if err := os.MkdirAll(bDir, 0755); err != nil {
		t.Fatalf("failed creating build dir: %v", err)
	}

	for _, path := range mockFiles { // create nested folder structure if present
		fullpath := filepath.Join(bDir, path)
		if err := os.MkdirAll(filepath.Dir(fullpath), 0755); err != nil {
			t.Fatalf("failed creating nested dir: %v", err)
		}
		if err := os.WriteFile(fullpath, []byte("file-data"), 0755); err != nil {
			t.Fatalf("failed creating mock file: %v", err)
		}
	}
	return zPath, wBin, bDir
}

func TestCreateBuildZip(t *testing.T) {
	tests := []struct {
		name, wrapperCfg string
		mockFiles, want  []string
	}{
		{"Zip creation simple", "config-string", []string{"game-binary.exe"}, []string{"amazon-gamelift-servers-game-server-wrapper", "config.yaml", "game-binary.exe"}},
		{"Zip creation nested structure", "config-string", []string{"Engine/Binaries/config.json"}, []string{"amazon-gamelift-servers-game-server-wrapper", "config.yaml", "Engine/", "Engine/Binaries/", "Engine/Binaries/config.json"}},
		{"Empty server build dir", "config-string", []string{}, []string{"amazon-gamelift-servers-game-server-wrapper", "config.yaml"}},
		{"multiple folders in server dir", "config-string", []string{"Engine/Binaries/config.json", "Engine/System/system.ini"}, []string{"amazon-gamelift-servers-game-server-wrapper", "config.yaml", "Engine/", "Engine/Binaries/", "Engine/Binaries/config.json", "Engine/System/", "Engine/System/system.ini"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			zPath, wBin, bDir := createTmpFilesys(t, tmp, tt.mockFiles) // create server build folder/files

			if err := createBuildZip(zPath, bDir, wBin, tt.wrapperCfg); err != nil { // build zip file
				t.Fatalf("Failed zip build: %v", err)
			}

			r, err := zip.OpenReader(zPath) // read zip file
			if err != nil {
				t.Fatalf("Failed opening tmp zipfile: %v", err)
			}
			defer r.Close()

			var files []string
			for _, f := range r.File {
				files = append(files, f.Name)
				if f.Name == "config.yaml" { // verify config.yaml contents
					rc, _ := f.Open()
					content, _ := io.ReadAll(rc)
					rc.Close()
					if string(content) != tt.wrapperCfg {
						t.Errorf("config mismatch: got %q, want %q", string(content), tt.wrapperCfg)
					}
				}
			}

			if !slices.Equal(files, tt.want) { // confirm .zip file contents
				t.Errorf("got %v, want %v", files, tt.want)
			}
		})
	}
}

func TestAddFileToZip(t *testing.T) {
	tests := []struct {
		name, newFile, zipPath string
		createMode, wantMode   os.FileMode
	}{
		{"Shell script forces 0755", "script.sh", "script.sh", 0644, 0755},
		{"Binary without ext forces 0755", "mybin", "bin/mybin", 0644, 0755},
		{"Text file preserves 0644", "config.json", "cfg/config.json", 0644, 0644},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			srcPath := filepath.Join(tmp, tt.newFile)
			_ = os.WriteFile(srcPath, []byte("file-data"), tt.createMode)

			zipFile := filepath.Join(tmp, "out.zip")
			f, err := os.Create(zipFile)
			if err != nil {
				t.Fatalf("failed to create zip file: %v", err)
			}

			w := zip.NewWriter(f)
			if err := addFileToZip(w, srcPath, tt.zipPath); err != nil { // create new zip file, writes to buffer
				t.Fatalf("Error adding files to zip: %v", err)
			}
			w.Close()
			f.Close()

			r, err := zip.OpenReader(zipFile) // read zip from buffer
			if err != nil {
				t.Fatalf("Failed opening tmp zipfile: %v", err)
			}
			defer r.Close()

			if len(r.File) != 1 { // ensure only the single file is added
				t.Fatalf("expected 1 file entry, got %d", len(r.File))
			}

			entry := r.File[0]
			if entry.Name != tt.zipPath { // verify zip file path
				t.Errorf("Naming mismatch: got %q, want %q", entry.Name, tt.zipPath)
			}
			if entry.Mode() != tt.wantMode {
				t.Errorf("Permissions mismatch: got %v, want %v", entry.Mode(), tt.wantMode)
			}
		})
	}
}

func TestBuildInput(t *testing.T) {
	d := Deployer{opts: DeployOptions{FleetName: "alpha-fleet"}}
	got := d.buildInput("s3bucket", "s3key", "AWSARN")

	checks := []struct{ field, got, want string }{
		{"Name", aws.ToString(got.Name), "ludus-alpha-fleet"},
		{"OS", string(got.OperatingSystem), string(gltypes.OperatingSystemAmazonLinux2023)},
		{"SDK", aws.ToString(got.ServerSdkVersion), "5.4.0"},
		{"Bucket", aws.ToString(got.StorageLocation.Bucket), "s3bucket"},
		{"Key", aws.ToString(got.StorageLocation.Key), "s3key"},
		{"RoleArn", aws.ToString(got.StorageLocation.RoleArn), "AWSARN"},
	}

	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s mismatch: got %q, want %q", c.field, c.got, c.want)
		}
	}

	tagMap := make(map[string]string)
	for _, tag := range got.Tags {
		tagMap[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}

	if tagMap["ludus:fleet-name"] != "alpha-fleet" || tagMap["ludus:target"] != "ec2" {
		t.Errorf("unexpected tags wired to CreateBuildInput: %v", tagMap)
	}
}

func TestGenerateEC2WrapperConfig(t *testing.T) {
	tests := []struct {
		name, binaryPath, mapPath string
		port                      int
		want                      string
	}{
		{
			name:       "Config formats correctly",
			binaryPath: "Engine/Binaries/gamelift.ini",
			mapPath:    "Engine/Binaries/config.json",
			port:       5432,
			want: `# Generated by ludus for GameLift Managed EC2
log-config:
  wrapper-log-level: debug
  game-server-logs-dir: ./game-server-logs

ports:
  gamePort: 5432

game-server-details:
  executable-file-path: Engine/Binaries/gamelift.ini
  game-server-args:
    - arg: "-port"
      val: "{{.GamePort}}"
      pos: 0
    - arg: "-Map=Engine/Binaries/config.json"
      pos: 1
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateEC2WrapperConfig(tt.binaryPath, tt.mapPath, tt.port)
			if got != tt.want {
				t.Errorf("generateEC2WrapperConfig mismatch:\ngot:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}
