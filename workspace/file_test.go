// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2024 Unstable Build, All Rights Reserved.
//
// NOTICE: All information contained herein is, and remains the property of COMPANY.
// The intellectual and technical concepts contained herein are proprietary to
// COMPANY and may be covered by U.S. and Foreign Patents, patents in process,
// and are protected by trade secret or copyright law. Dissemination of this information
// or reproduction of this material is strictly forbidden unless prior written permission
// is obtained from COMPANY. Access to the source code contained herein is hereby
// forbidden to anyone except current COMPANY employees, managers or contractors who
// have executed Confidentiality and Non-disclosure agreements explicitly covering such access.
//
// The copyright notice above does not evidence any actual or intended publication or
// disclosure of this source code, which includes information that is confidential and/or
// proprietary, and is a trade secret, of COMPANY. ANY REPRODUCTION, MODIFICATION,
// DISTRIBUTION, PUBLIC  PERFORMANCE, OR PUBLIC DISPLAY OF OR THROUGH USE OF THIS SOURCE CODE
// WITHOUT  THE EXPRESS WRITTEN CONSENT OF COMPANY IS STRICTLY PROHIBITED, AND IN
// VIOLATION OF APPLICABLE LAWS AND INTERNATIONAL TREATIES. THE RECEIPT OR POSSESSION OF
// THIS SOURCE CODE AND/OR RELATED INFORMATION DOES NOT CONVEY OR IMPLY ANY RIGHTS TO
// REPRODUCE, DISCLOSE OR DISTRIBUTE ITS CONTENTS, OR TO MANUFACTURE, USE, OR SELL
// ANYTHING THAT IT MAY DESCRIBE, IN WHOLE OR IN PART.

package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	os "os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	gomock "go.uber.org/mock/gomock"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/workspace/workspaceapitest"
)

const sampleSnippet = `
/*
 * Check if the current buffer should be added to or removed from the list of
 * diff buffers.
 */
	void
diff_buf_adjust(win_T *win)
{
	win_T	*wp;
	int		i;

	if (!win->w_p_diff)
	{
	/* When there is no window showing a diff for this buffer, remove
	 * it from the diffs. */
	FOR_ALL_WINDOWS(wp)
		if (wp->w_buffer == win->w_buffer && wp->w_p_diff)
		break;
	if (wp == NULL)
	{
		i = diff_buf_idx(win->w_buffer);
		if (i != DB_COUNT)
		{
		curtab->tp_diffbuf[i] = NULL;
		curtab->tp_diff_invalid = TRUE;
		diff_redraw(TRUE);
		}
	}
	}
	else
	diff_buf_add(win->w_buffer);
} /* { */ `

// INTEGRATION TESTS

func newIntegrationTestCase(t *testing.T, endsInEOL bool) (
	*cell.Buffer, *os.File,
) {
	buffer := cell.NewBuffer()
	file, err := os.CreateTemp("", "frctl_file_test")
	require.NoError(t, err)

	_, err = file.Write([]byte(sampleSnippet))
	require.NoError(t, err)

	if endsInEOL {
		_, err = file.Write([]byte{'\n'})
		require.NoError(t, err)
	}
	t.Cleanup(func() {
		_ = file.Close()
		_ = os.Remove(file.Name())
	})
	return buffer, file
}

// refactor shim
func openFile(
	filename string, buf *cell.Buffer, swapDir string, readOnly bool,
) (*file, error) {
	workspaceURI, err := makeLocalURI(filepath.Dir(filename))
	if err != nil {
		return nil, err
	}
	scheme, err := newTestFileScheme(workspaceURI)
	if err != nil {
		return nil, err
	}
	l, err := newFile(scheme, filename, buf, swapDir, readOnly)
	if err != nil {
		return nil, err
	}
	return l, nil
}

// refactor shim
func recoverFile(
	filename, recoverFilename string, buf *cell.Buffer, force bool,
) (*file, error) {
	workspaceURI, err := makeLocalURI(filepath.Dir(filename))
	if err != nil {
		return nil, err
	}
	scheme, err := newTestFileScheme(workspaceURI)
	if err != nil {
		return nil, err
	}
	l, err := newFileRecover(scheme, filename, recoverFilename, buf, force)
	if err != nil {
		return nil, err
	}
	return l, nil
}

// tests FileBuffer with real os.File's. endsInEOL refers to the original file.
func testFileBufferIntegration(t *testing.T, endsInEOL bool) {
	t.Run("if file does not exist, create it upon Flush", func(t *testing.T) {
		buf := cell.NewBuffer()
		file, err := os.CreateTemp("", "frctl_file_test")
		require.NoError(t, err)

		// secure a random filename in a tmp directory
		filename := file.Name()
		require.NoError(t, os.Remove(filename))

		f, err := openFile(filename, buf, "", false)
		require.NoError(t, err)
		defer f.Close()

		_, err = os.Stat(filename)
		require.Error(t, err)

		require.NoError(t, f.Flush())

		_, err = os.Stat(filename)
		require.NoError(t, err)
	})

	t.Run("if file does not exist and readOnly, error out", func(t *testing.T) {
		buf := cell.NewBuffer()
		file, err := os.CreateTemp("", "frctl_file_test")
		require.NoError(t, err)

		// secure a random filename in a tmp directory
		filename := file.Name()
		require.NoError(t, os.Remove(filename))

		_, err = openFile(filename, buf, "", true)
		require.Error(t, err)

		_, err = os.Stat(filename)
		require.Error(t, err)
	})

	t.Run("if readOnly, error out on flush", func(t *testing.T) {
		buf := cell.NewBuffer()
		file, err := os.CreateTemp("", "frctl_file_test")
		require.NoError(t, err)

		filename := file.Name()
		os.WriteFile(filename, []byte("blah"), 0000)
		require.NoError(t, file.Close())

		f, err := openFile(filename, buf, "", true)
		require.NoError(t, err)
		defer f.Close()

		buf.WriteString("meh")
		assert.Error(t, f.Flush())

		data, err := os.ReadFile(filename)
		require.NoError(t, err)
		assert.Equal(t, "blah", string(data))
	})

	t.Run("if readOnly no swap files are initialized", func(t *testing.T) {
		buf := cell.NewBuffer()
		file, err := os.CreateTemp("", "frctl_file_test")
		require.NoError(t, err)

		filename := file.Name()
		require.NoError(t, file.Close())

		swapDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		f, err := openFile(filename, buf, swapDir, true)
		require.NoError(t, err)
		defer f.Close()

		_, swapFileName := swapFileName(swapDir, file.Name())

		_, err = os.Stat(swapFileName)
		require.Error(t, err)
	})

	t.Run("if readOnly it doesn't error out if file is already open", func(t *testing.T) {
		buf := cell.NewBuffer()
		file, err := os.CreateTemp("", "frctl_file_test")
		require.NoError(t, err)

		filename := file.Name()
		require.NoError(t, file.Close())

		f1, err := openFile(filename, buf, "", false)
		require.NoError(t, err)
		defer f1.Close()

		f2, err := openFile(filename, buf, "", true)
		require.NoError(t, err)
		defer f2.Close()
	})

	t.Run("if swap holding buffer is closed, then NewFileBuffer should NOT error out", func(t *testing.T) {
		buf := cell.NewBuffer()
		file, err := os.CreateTemp("", "frctl_file_test")
		require.NoError(t, err)

		filename := file.Name()
		require.NoError(t, file.Close())

		f1, err := openFile(filename, buf, "", false)
		require.NoError(t, err)

		_, err = openFile(filename, buf, "", false)
		require.Error(t, err)

		assert.NoError(t, f1.Close())

		f2, err := openFile(filename, buf, "", false)
		require.NoError(t, err)
		assert.NoError(t, f2.Close())
	})

	t.Run("respects original file mode", func(t *testing.T) {
		buf := cell.NewBuffer()
		file, err := os.CreateTemp("", "frctl_file_test")
		require.NoError(t, err)

		filename := file.Name()

		err = file.Chmod(0700)
		require.NoError(t, err)
		require.NoError(t, file.Close())

		f, err := openFile(filename, buf, "", false)
		require.NoError(t, err)
		defer f.Close()

		fileInfo, err := os.Stat(filename)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0700), fileInfo.Mode())

		require.NoError(t, f.Flush())

		fileInfo, err = os.Stat(filename)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0700), fileInfo.Mode())
	})

	t.Run("respects symlinks", func(t *testing.T) {
		buf := cell.NewBuffer()
		orig, err := os.CreateTemp("", "frctl_file_test")
		require.NoError(t, err)

		require.NoError(t, orig.Chmod(0700))
		filename := orig.Name() + ".symlink"

		require.NoError(t, os.Symlink(orig.Name(), filename))
		fileInfo, err := os.Lstat(filename)
		require.NoError(t, err)
		assert.True(t, fileInfo.Mode()&os.ModeSymlink == os.ModeSymlink)

		require.NoError(t, orig.Close())

		f, err := openFile(filename, buf, "", false)
		require.NoError(t, err)
		defer f.Close()
		require.NoError(t, f.Flush())
		b, err := os.ReadFile(filename)
		require.NoError(t, err)
		assert.Equal(t, "", string(b))

		buf.InsertString(term.Coordinates{}, "blah\n")

		require.NoError(t, f.Flush())

		fileInfo, err = os.Lstat(filename)
		require.NoError(t, err)
		assert.True(t, fileInfo.Mode()&os.ModeSymlink == os.ModeSymlink)

		b, err = os.ReadFile(filename)
		require.NoError(t, err)
		assert.Equal(t, "blah\n", string(b))

		fileInfo, err = os.Stat(orig.Name())
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0700), fileInfo.Mode())
	})

	t.Run("if file does not exist, if there are errors upon creation, it bubbles up on Flush", func(t *testing.T) {
		buf := cell.NewBuffer()
		rgen := rand.New(rand.NewSource(int64(time.Now().Nanosecond())))
		filename := fmt.Sprintf("/tmp/mpo/tmp/tmp/tmp/tmp/%d.go", rgen.Int())

		f, err := openFile(filename, buf, os.TempDir(), false)
		require.NoError(t, err)
		defer f.Close()

		_, err = os.Stat(filename)
		require.Error(t, err)

		require.Error(t, f.Flush())
	})

	t.Run("if file does not exist, Reload errors", func(t *testing.T) {
		buf := cell.NewBuffer()
		rgen := rand.New(rand.NewSource(int64(time.Now().Nanosecond())))
		filename := fmt.Sprintf("/tmp/%d.go", rgen.Int())

		f, err := openFile(filename, buf, os.TempDir(), false)
		require.NoError(t, err)
		defer f.Close()

		_, err = os.Stat(filename)
		require.Error(t, err)

		require.Error(t, f.Reload())
	})

	t.Run("no swap file is open, creates one; removes on close", func(t *testing.T) {
		b, file := newIntegrationTestCase(t, endsInEOL)

		swapDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		_, swapFileName := swapFileName(swapDir, file.Name())

		_, err = os.Stat(swapFileName)
		require.Error(t, err)

		f, err := openFile(file.Name(), b, swapDir, false)
		require.NoError(t, err)

		_, err = os.Stat(swapFileName)
		require.NoError(t, err, swapFileName)

		f.Close()

		_, err = os.Stat(swapFileName)
		assert.Error(t, err)
	})

	t.Run("fsyncs swap upon update", func(t *testing.T) {
		for _, reload := range []bool{false, true} {
			b, file := newIntegrationTestCase(t, endsInEOL)

			f, err := openFile(file.Name(), b, "", false)
			require.NoError(t, err)

			if reload {
				require.NoError(t, f.Reload())
			}

			buf, err := os.ReadFile(f.swap.Name())
			require.NoError(t, err)
			content := sampleSnippet
			if endsInEOL {
				content += "\n"
			}
			assert.Equal(t, content, string(buf))

			const writeStr = "XXXXXX"
			b.InsertString(term.Coordinates{}, writeStr)

			// wait for updates
			f.wg.Wait()

			buf, err = os.ReadFile(f.swap.Name())
			require.NoError(t, err)

			assert.Equal(t, writeStr+sampleSnippet+"\n", string(buf))
		}
	})

	t.Run("fsyncs file upon Flush", func(t *testing.T) {
		b, file := newIntegrationTestCase(t, endsInEOL)

		f, err := openFile(file.Name(), b, "", false)
		require.NoError(t, err)
		defer f.Close()

		const writeStr = "XXXXXX"
		b.InsertString(term.Coordinates{}, writeStr)
		require.NoError(t, f.Flush())

		buf, err := os.ReadFile(file.Name())
		require.NoError(t, err)

		assert.Equal(t, writeStr+sampleSnippet+"\n", string(buf))
	})

	t.Run("returns error if swap is already open", func(t *testing.T) {
		b, file := newIntegrationTestCase(t, endsInEOL)

		swapDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		f, err := openFile(file.Name(), b, swapDir, false)
		require.NoError(t, err)
		defer f.Close()

		_, err = openFile(file.Name(), b, swapDir, false)
		assert.Equal(t, workspaceapi.ErrFileAlreadyOpen, err)
	})

	t.Run("Reload reloads file as it was originally on disk", func(t *testing.T) {
		b, file := newIntegrationTestCase(t, endsInEOL)

		f, err := openFile(file.Name(), b, "", false)
		require.NoError(t, err)

		const writeStr = "XXXXXX"
		b.InsertString(term.Coordinates{}, writeStr)

		require.NoError(t, f.Reload())
		assertFileAndBufferOnDisk(t, b, file.Name(), sampleSnippet, endsInEOL)
		require.NoError(t, f.Close())
	})

	t.Run("Reload reloads file as it was on disk, after Flush", func(t *testing.T) {
		b, file := newIntegrationTestCase(t, endsInEOL)

		f, err := openFile(file.Name(), b, "", false)
		require.NoError(t, err)

		const writeStr = "XXXXXX"
		b.InsertString(term.Coordinates{}, writeStr)

		require.NoError(t, f.Flush())
		assertFileAndBufferOnDisk(t, b, file.Name(), writeStr+sampleSnippet, true)

		require.NoError(t, f.Reload())
		assertFileAndBufferOnDisk(t, b, file.Name(), writeStr+sampleSnippet, true)

		b.InsertString(term.Coordinates{}, writeStr)
		require.NoError(t, f.Reload())
		assertFileAndBufferOnDisk(t, b, file.Name(), writeStr+sampleSnippet, true)
	})

	t.Run("Reload reloads originally uncreated file", func(t *testing.T) {
		b, file := newIntegrationTestCase(t, endsInEOL)
		require.NoError(t, file.Close())
		require.NoError(t, os.Remove(file.Name()))

		f, err := openFile(file.Name(), b, "", false)
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(file.Name(), []byte("deep purple\n"), 0666))

		require.NoError(t, f.Reload())
		assertFileAndBufferOnDisk(t, b, file.Name(), "deep purple", true)
	})

	t.Run("Reload integration with cell subscribers", func(t *testing.T) {
		b, file := newIntegrationTestCase(t, endsInEOL)

		f, err := openFile(file.Name(), b, "", false)
		require.NoError(t, err)

		buf := cell.NewBuffer()
		b.Subscribe(&testCellSubscriber{buf: buf})

		const writeStr = "XXXXXX"
		b.InsertString(term.Coordinates{}, writeStr)

		require.NoError(t, f.Reload())
		assertFileAndBufferOnDisk(t, b, file.Name(), sampleSnippet, endsInEOL)
		assert.Equal(t, sampleSnippet+"\n", buf.String()) // buf doesn't have unix view

		require.NoError(t, f.Close())
	})
}

type testCellSubscriber struct {
	buf *cell.Buffer
}

func (t *testCellSubscriber) OnWillEdit(
	ctx context.Context, start, end term.Coordinates, str string,
) {
	t.buf.Edit(ctx, start, end, str)
}

func (t *testCellSubscriber) OnDidEdit(
	ctx context.Context, from, to term.Coordinates, old string,
) {
}

func assertFileAndBufferOnDisk(
	t *testing.T, b *cell.Buffer, name, expected string,
	expectedLastEOL bool,
) {
	t.Helper()

	assert.Equal(t, expected, b.String())

	data, err := os.ReadFile(name)
	require.NoError(t, err)

	expectedOnDisk := expected
	if expectedLastEOL {
		expectedOnDisk += "\n"
	}
	assert.Equal(t, expectedOnDisk, string(data))
}

func TestFileBufferIntegrationEOL(t *testing.T) {
	testFileBufferIntegration(t, true)
}

func TestFileBufferIntegrationNOEOL(t *testing.T) {
	testFileBufferIntegration(t, false)
}

func assertRecoverFromSwapFile(t *testing.T, filename, swapname string, b *cell.Buffer) {
	assert.Equal(t, sampleSnippet, b.String())

	buf, err := os.ReadFile(filename)
	require.NoError(t, err)
	assert.Equal(t, sampleSnippet+"\n", string(buf))

	buf, err = os.ReadFile(swapname)
	require.NoError(t, err)
	assert.Equal(t, sampleSnippet+"\n", string(buf))
}

func newRecoveryIntegrationCase(t *testing.T) (
	*cell.Buffer, *os.File, *os.File,
) {
	_, file := newIntegrationTestCase(t, true)
	require.NoError(t, file.Truncate(0))

	b, swap := newIntegrationTestCase(t, true)
	return b, file, swap
}

func TestFileBufferRecover(t *testing.T) {
	t.Run("recovers file from swap", func(t *testing.T) {
		b, file, swap := newRecoveryIntegrationCase(t)

		filepath, swapFilepath := file.Name(), swap.Name()
		f, err := recoverFile(filepath, swapFilepath, b, false)
		require.NoError(t, err)
		defer f.Close()

		assertRecoverFromSwapFile(t, filepath, swapFilepath, b)
	})

	t.Run("recovers file from swap even if file does not exist", func(t *testing.T) {
		b, swap := newIntegrationTestCase(t, true)

		swapFilepath := swap.Name()
		swapFileName := path.Base(swapFilepath)
		filepath := path.Join(path.Dir(swapFilepath), "my_actual_file"+swapFileName)
		f, err := recoverFile(filepath, swapFilepath, b, false)
		require.NoError(t, err, filepath)
		defer f.Close()

		assertRecoverFromSwapFile(t, filepath, swapFilepath, b)
	})

	t.Run("returns error if file was modified after swap and does not remove swap", func(t *testing.T) {
		b, file, swap := newRecoveryIntegrationCase(t)

		filepath, swapFilepath := file.Name(), swap.Name()

		time.Sleep(10 * time.Millisecond)
		_, err := file.WriteString("blah")
		require.NoError(t, err)
		require.NoError(t, file.Sync())

		_, err = recoverFile(filepath, swapFilepath, b, false)
		assert.Equal(t, workspaceapi.ErrStaleData, err)

		_, err = os.Stat(swapFilepath)
		require.NoError(t, err)
	})

	t.Run("recovers if file was modified after swap and recover was called with force=true", func(t *testing.T) {
		b, file, swap := newRecoveryIntegrationCase(t)

		filepath, swapFilepath := file.Name(), swap.Name()

		time.Sleep(10 * time.Millisecond)
		_, err := file.WriteString("Inma")
		require.NoError(t, err)
		require.NoError(t, file.Sync())

		_, err = recoverFile(filepath, swapFilepath, b, true)
		assert.NoError(t, err)

		assertRecoverFromSwapFile(t, filepath, swapFilepath, b)
	})

	t.Run("recovers file and updates it with swap contents if swap is ahead", func(t *testing.T) {
		b, file, swap := newRecoveryIntegrationCase(t)

		filepath, swapFilepath := file.Name(), swap.Name()

		time.Sleep(10 * time.Millisecond)
		_, err := swap.WriteString("blah")
		require.NoError(t, err)
		require.NoError(t, swap.Sync())

		swapContent, err := os.ReadFile(swapFilepath)
		require.NoError(t, err)

		_, err = recoverFile(filepath, swapFilepath, b, false)
		require.NoError(t, err)

		fileContent, err := os.ReadFile(filepath)
		require.NoError(t, err)
		assert.Equal(t, swapContent, fileContent)
	})

	t.Run("if a recover buffer takes over swap, it should now allow for other NewFileBuffer to open it", func(t *testing.T) {
		b, file, _ := newRecoveryIntegrationCase(t)

		swapDir := filepath.Dir(file.Name())

		f1, err := openFile(file.Name(), b, swapDir, false)
		require.NoError(t, err)
		defer f1.Close()

		// checking update time is time based
		time.Sleep(10 * time.Millisecond)

		b2 := cell.NewBuffer()
		f2, err := recoverFile(file.Name(), f1.swapFileName, b2, false)
		require.NoError(t, err)
		defer f2.Close()

		time.Sleep(10 * time.Millisecond)

		require.Equal(t, workspaceapi.ErrStaleData, f1.Flush())

		_, err = openFile(file.Name(), b, swapDir, false)
		require.Error(t, err)
	})
}

func TestForceFlush(t *testing.T) {
	t.Run("flushes to disk", func(t *testing.T) {
		b, file := newIntegrationTestCase(t, true)

		f, err := openFile(file.Name(), b, "", false)
		require.NoError(t, err)

		const writeStr = "XXXXXX"
		b.InsertString(term.Coordinates{}, writeStr)

		require.NoError(t, f.ForceFlush())
		assertFileAndBufferOnDisk(t, b, file.Name(), writeStr+sampleSnippet, true)
	})

	t.Run("flushes to disk after a reload", func(t *testing.T) {
		b, file := newIntegrationTestCase(t, true)

		f, err := openFile(file.Name(), b, "", false)
		require.NoError(t, err)

		const writeStr = "XXXXXX"
		b.InsertString(term.Coordinates{}, writeStr)

		require.NoError(t, f.Reload())
		require.NoError(t, f.ForceFlush())
		assertFileAndBufferOnDisk(t, b, file.Name(), sampleSnippet, true)
	})

	t.Run("is able to overwrite when a file was modified oob", func(t *testing.T) {
		b, file := newIntegrationTestCase(t, true)

		f, err := openFile(file.Name(), b, "", false)
		require.NoError(t, err)

		const writeStr = "XXXXXX"
		b.InsertString(term.Coordinates{}, writeStr)

		require.NoError(t, f.Flush())
		assertFileAndBufferOnDisk(t, b, file.Name(), writeStr+sampleSnippet, true)

		require.NoError(t, os.WriteFile(file.Name(), []byte("abv"), 0))
		data, err := os.ReadFile(file.Name())
		require.NoError(t, err)
		assert.Equal(t, "abv", string(data))

		require.NoError(t, f.ForceFlush())
		assertFileAndBufferOnDisk(t, b, file.Name(), writeStr+sampleSnippet, true)
	})

	t.Run("creates file if file is removed oob", func(t *testing.T) {
		b, file := newIntegrationTestCase(t, true)

		f, err := openFile(file.Name(), b, "", false)
		require.NoError(t, err)

		const writeStr = "XXXXXX"
		b.InsertString(term.Coordinates{}, writeStr)

		require.NoError(t, f.Flush())
		assertFileAndBufferOnDisk(t, b, file.Name(), writeStr+sampleSnippet, true)

		require.NoError(t, os.Remove(file.Name()))

		require.NoError(t, f.ForceFlush())
		assertFileAndBufferOnDisk(t, b, file.Name(), writeStr+sampleSnippet, true)
	})

	t.Run("overwrites read-only", func(t *testing.T) {
		b, file := newIntegrationTestCase(t, true)

		f, err := openFile(file.Name(), b, "", true)
		require.NoError(t, err)

		const writeStr = "XXXXXX"
		b.InsertString(term.Coordinates{}, writeStr)

		require.NoError(t, f.ForceFlush())
		assertFileAndBufferOnDisk(t, b, file.Name(), writeStr+sampleSnippet, true)
	})

	t.Run("overwrites read-only no changes, respects original contents", func(t *testing.T) {
		b, file := newIntegrationTestCase(t, true)

		f, err := openFile(file.Name(), b, "", true)
		require.NoError(t, err)

		require.NoError(t, f.ForceFlush())
		assertFileAndBufferOnDisk(t, b, file.Name(), sampleSnippet, true)
	})

	t.Run("Flush after ForceFlush a readonly should not error", func(t *testing.T) {
		b, file := newIntegrationTestCase(t, true)

		f, err := openFile(file.Name(), b, "", true)
		require.NoError(t, err)

		const writeStr = "XXXXXX"
		b.InsertString(term.Coordinates{}, writeStr)

		require.NoError(t, f.ForceFlush())
		assertFileAndBufferOnDisk(t, b, file.Name(), writeStr+sampleSnippet, true)

		b.InsertString(term.Coordinates{}, writeStr)

		require.NoError(t, f.Flush())
		assertFileAndBufferOnDisk(t, b, file.Name(), writeStr+writeStr+sampleSnippet, true)
	})
}

// UNIT TESTS

// returns an un-initialized (but dep injected) FileBuffer along with the mocked OsFile
func newTestFileBuffer(ctrl *gomock.Controller) (*file, *workspaceapitest.MockFile) {
	schemeIfc, _ := newTestScheme("test")(context.Background(), nil, workspaceapi.URI{})
	scheme := schemeIfc.(*testScheme)
	mock := workspaceapitest.NewMockFile(ctrl)
	scheme.openFunc = func(name string, flag int, perm os.FileMode) (workspaceapi.File, error) {
		return mock, nil
	}
	f := new(file)
	f.scheme = scheme
	return f, mock
}

func expectRead(mock *workspaceapitest.MockFile, data []byte) {
	mock.EXPECT().Read(gomock.Any()).DoAndReturn(func(buf []byte) (int, error) {
		if len(buf) < len(data) {
			panic("seriously?")
		}
		n := copy(buf, data)
		return n, io.EOF
	})
}

func expectInitSwap(
	mock *workspaceapitest.MockFile, fileName string, fileInfo os.FileInfo, data []byte,
) {
	// stat original file
	mock.EXPECT().Stat().Return(fileInfo, nil).AnyTimes()
	// read original file
	expectRead(mock, data)

	mock.EXPECT().Name().Return(fileName).AnyTimes()
	// seek original file back to 0
	mock.EXPECT().Seek(gomock.Eq(int64(0)), gomock.Eq(0)).Return(int64(0), nil)

	mock.EXPECT().Truncate(gomock.Any()).Return(nil)

	// write to swap file
	mock.EXPECT().Write(gomock.Any()).DoAndReturn(func(p []byte) (n int, err error) {
		// assert.EqualValues(t, data, p)
		return len(p), nil
	})
	mock.EXPECT().Sync().Return(nil)
}

func expectInitBuffer(mock *workspaceapitest.MockFile, data []byte) {
	expectRead(mock, data)

	// seek original file back to 0
	mock.EXPECT().Seek(gomock.Eq(int64(0)), gomock.Eq(0)).Return(int64(0), nil)
}

func TestFileBufferInit(t *testing.T) {
	t.Run("is able to use a symlink file", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		fileName := "elMeuNom"
		data := []byte("bon dia senyor")
		fileInfo := testFileInfo{mode: os.ModeSymlink}

		f, mock := newTestFileBuffer(ctrl)

		expectInitSwap(mock, fileName, fileInfo, data)
		expectInitBuffer(mock, data)

		assert.NoError(t, f.init(fileName, cell.NewBuffer(), "", false))
	})

	t.Run("is able to use a regular file", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		fileName := "myName"
		data := []byte("good morning sir")
		fileInfo := testFileInfo{}

		f, mock := newTestFileBuffer(ctrl)
		expectInitSwap(mock, fileName, fileInfo, data)
		expectInitBuffer(mock, data)

		assert.NoError(t, f.init(fileName, cell.NewBuffer(), "", false))
	})

	t.Run("masks whether there's a last EOL from clients", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		fileName := "CA"

		for _, data := range [][]byte{[]byte("Oakland\n"), []byte("Oakland")} {
			fileInfo := testFileInfo{}

			f, mock := newTestFileBuffer(ctrl)
			expectInitSwap(mock, fileName, fileInfo, data)
			expectInitBuffer(mock, data)

			buf := cell.NewBuffer()
			assert.NoError(t, f.init(fileName, buf, "", false))
			assert.Equal(t, "Oakland", buf.String(), fmt.Sprintf("%q", string(data)))
			assert.Equal(t, 1, buf.Rows(), fmt.Sprintf("%q", string(data)))

			wait := expectCopyToSwapPrepare(f, mock)
			mock.EXPECT().
				Write(gomock.Any()).
				Return(1, nil)
			mock.EXPECT().Sync().Return(nil)
			buf.WriteString("\n")
			assert.Equal(t, "Oakland\n", buf.String(), fmt.Sprintf("%q", string(data)))
			assert.Equal(t, 2, buf.Rows(), fmt.Sprintf("%q", string(data)))
			wait()
		}
	})

	t.Run("returns error if original file is not regular or symlink file", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		f, mock := newTestFileBuffer(ctrl)
		mock.EXPECT().Stat().Return(testFileInfo{mode: os.ModeSocket}, nil)

		assert.Equal(t, workspaceapi.ErrFileIsNotRegular, f.init("fjkelw", cell.NewBuffer(), "", false))
	})

	t.Run("returns error if original file is directory", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		f, mock := newTestFileBuffer(ctrl)
		mock.EXPECT().Stat().Return(testFileInfo{isDir: true}, nil)

		assert.Equal(t, workspaceapi.ErrFileIsNotRegular, f.init("fjkelw", cell.NewBuffer(), "", false))
	})

	t.Run("bubble up original file open error", func(t *testing.T) {
		accessDeniedErr := errors.New("access denied")
		f := new(file)
		f.scheme = &testScheme{}
		f.scheme.(*testScheme).openFunc = func(name string, flag int, perm os.FileMode) (workspaceapi.File, error) {
			return nil, accessDeniedErr
		}
		assert.Equal(t, accessDeniedErr, f.init("fjkelw", cell.NewBuffer(), "", false))
	})

	t.Run("bubble up swap file open error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		accessDeniedErr := errors.New("access denied")
		origFileMock := workspaceapitest.NewMockFile(ctrl)
		f := new(file)
		f.scheme = &testScheme{}
		i := 0
		f.scheme.(*testScheme).openFunc = func(name string, flag int, perm os.FileMode) (workspaceapi.File, error) {
			i++
			if i == 1 {
				return origFileMock, nil
			}
			return nil, accessDeniedErr
		}

		fileName := "fjklewjflk"

		origFileMock.EXPECT().Stat().Return(testFileInfo{}, nil)
		origFileMock.EXPECT().Name().Return(fileName).AnyTimes()

		assert.Equal(t, accessDeniedErr, f.init(fileName, cell.NewBuffer(), "", false))
	})

	t.Run("bubble up swap file write error, and remove empty file swap", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		accessDeniedErr := errors.New("access denied")
		fileName := "myName"
		data := []byte("good morning sir")
		fileInfo := testFileInfo{}

		f, mock := newTestFileBuffer(ctrl)
		var i int
		f.scheme.(*testScheme).removeFunc = func(name string) error {
			i++
			return nil
		}
		mock.EXPECT().Stat().Return(fileInfo, nil).AnyTimes()
		expectRead(mock, data)

		mock.EXPECT().Name().Return(fileName).AnyTimes()

		mock.EXPECT().Truncate(gomock.Any()).Return(nil)

		mock.EXPECT().Write(gomock.Any()).DoAndReturn(func(p []byte) (n int, err error) {
			return 0, accessDeniedErr
		})

		assert.Error(t, f.init(fileName, cell.NewBuffer(), "", false))
		assert.Equal(t, 1, i)
	})

	t.Run("bubble up swap file read error, and remove empty file swap", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		accessDeniedErr := errors.New("access denied")
		fileName := "myName"
		fileInfo := testFileInfo{}

		f, mock := newTestFileBuffer(ctrl)
		var i int
		f.scheme.(*testScheme).removeFunc = func(name string) error {
			i++
			return nil
		}
		mock.EXPECT().Stat().Return(fileInfo, nil).AnyTimes()
		mock.EXPECT().Read(gomock.Any()).DoAndReturn(func(buf []byte) (int, error) {
			return 0, accessDeniedErr
		})

		assert.Error(t, f.init(fileName, cell.NewBuffer(), "", false))
		assert.Equal(t, 1, i)
	})

	t.Run("bubble up swap file stat error, and remove empty file swap", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		accessDeniedErr := errors.New("access denied")
		fileName := "myName"
		fileInfo := testFileInfo{}

		f, mock := newTestFileBuffer(ctrl)
		var i int
		f.scheme.(*testScheme).removeFunc = func(name string) error {
			i++
			return nil
		}
		var j int
		mock.EXPECT().Stat().Return(fileInfo, nil).DoAndReturn(func() (os.FileInfo, error) {
			j++
			if j == 1 {
				return fileInfo, nil
			}
			return nil, accessDeniedErr
		})
		expectRead(mock, []byte("1234"))

		mock.EXPECT().Name().Return(fileName).AnyTimes()
		mock.EXPECT().Truncate(gomock.Any()).Return(nil)

		mock.EXPECT().Write(gomock.Any()).DoAndReturn(func(p []byte) (n int, err error) {
			return 0, accessDeniedErr
		})

		assert.Error(t, f.init(fileName, cell.NewBuffer(), "", false))
		assert.Equal(t, 1, i)
	})
}

const defaultFileName = "myOhDear.go"

var defaultFileData = []byte("oh, dear")

func newInitializedTestFileBuffer(t *testing.T, ctrl *gomock.Controller) (
	*file, *workspaceapitest.MockFile, *cell.Buffer,
) {
	f, mock := newTestFileBuffer(ctrl)
	expectInitSwap(mock, defaultFileName, testFileInfo{}, defaultFileData)
	expectInitBuffer(mock, defaultFileData)
	buf := cell.NewBuffer()
	require.NoError(t, f.init(defaultFileName, buf, "", false))
	return f, mock, buf
}

type testOsError struct {
	isExistErr      bool
	isNotExistErr   bool
	isPermissionErr bool
}

func (t testOsError) isPermission() bool {
	return t.isPermissionErr
}
func (t testOsError) isExist() bool {
	return t.isExistErr
}
func (t testOsError) isNotExist() bool {
	return t.isNotExistErr
}

func newUninitializedTestFileBuffer(t *testing.T, ctrl *gomock.Controller) (
	*file, *workspaceapitest.MockFile, *cell.Buffer,
) {
	f, mock := newTestFileBuffer(ctrl)
	f.scheme.(*testScheme).openFunc = func(name string, flag int, perm os.FileMode) (workspaceapi.File, error) {
		if flag&os.O_CREATE != 0 {
			mock.EXPECT().Read(gomock.Any()).Return(0, io.EOF).Times(1)
			mock.EXPECT().Seek(gomock.Any(), gomock.Any()).Return(int64(0), nil).Times(1)
			return mock, nil
		}
		return nil, os.ErrNotExist
	}

	mock.EXPECT().Name().Return(defaultFileName).AnyTimes()

	buf := cell.NewBuffer()
	require.NoError(t, f.init(defaultFileName, buf, "", false))
	return f, mock, buf
}

func newReadOnlyTestFileBuffer(t *testing.T, ctrl *gomock.Controller) (
	*file, *workspaceapitest.MockFile, *cell.Buffer,
) {
	f, mock := newTestFileBuffer(ctrl)
	f.scheme.(*testScheme).openFunc = func(name string, flag int, perm os.FileMode) (workspaceapi.File, error) {
		if flag&os.O_RDWR != 0 || flag&os.O_CREATE != 0 {
			return nil, os.ErrPermission
		}
		return mock, nil
	}

	mock.EXPECT().Name().Return(defaultFileName).AnyTimes()
	mock.EXPECT().Stat().Return(testFileInfo{}, nil).AnyTimes()
	expectRead(mock, defaultFileData)
	mock.EXPECT().Seek(gomock.Eq(int64(0)), gomock.Eq(0)).Return(int64(0), nil)

	buf := cell.NewBuffer()
	require.NoError(t, f.init(defaultFileName, buf, "", false))
	return f, mock, buf
}

func newRecoveredTestFileBuffer(t *testing.T, ctrl *gomock.Controller) (
	*file, *workspaceapitest.MockFile, *cell.Buffer,
) {
	f, mock := newTestFileBuffer(ctrl)
	mock.EXPECT().Stat().Return(testFileInfo{}, nil).AnyTimes()
	mock.EXPECT().Close().Return(nil).Times(2)
	expectInitSwap(mock, defaultFileName, testFileInfo{}, defaultFileData)
	expectInitBuffer(mock, defaultFileData)
	buf := cell.NewBuffer()
	require.NoError(t, f.initRecover(defaultFileName,
		"."+defaultFileName+".swp", buf, false))
	return f, mock, buf
}

func testFileBufferClose(t *testing.T, newBuffer newBufferFunc) {
	t.Run("Close should remove and close all resources such that Init can be called again", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		f, mock, _ := newBuffer(t, ctrl)

		mock.EXPECT().Close().Return(nil).Times(2)
		assert.NoError(t, f.Close())

		expectInitSwap(mock, defaultFileName, testFileInfo{}, defaultFileData)
		expectInitBuffer(mock, defaultFileData)
		assert.NoError(t, f.init(defaultFileName, cell.NewBuffer(), "", false))
	})

	t.Run("two consecutive calls to Close should return an error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		f, mock, _ := newBuffer(t, ctrl)

		mock.EXPECT().Close().Return(nil).Times(2)
		assert.NoError(t, f.Close())
		assert.Error(t, f.Close())
	})

	t.Run("Close should remove the swap file", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		f, mock, _ := newBuffer(t, ctrl)
		called := false
		f.scheme.(*testScheme).removeFunc = func(name string) error {
			called = true
			return nil
		}

		mock.EXPECT().Close().Return(nil).Times(2)
		assert.NoError(t, f.Close())
		assert.True(t, called)
	})

	t.Run("Close returns no errors if file was opened in read-only", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		f, mock, _ := newReadOnlyTestFileBuffer(t, ctrl)
		mock.EXPECT().Close().Return(nil).Times(1)
		assert.NoError(t, f.Close())
	})
}

func TestNewFileBufferClose(t *testing.T) {
	testFileBufferClose(t, newInitializedTestFileBuffer)
}

func TestRecoverFileBufferClose(t *testing.T) {
	testFileBufferClose(t, newRecoveredTestFileBuffer)
}

func TestFileNotCreatedBufferClose(t *testing.T) {
	t.Run("Close should remove and close all resources such that Init can be called again", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		f, mock, _ := newUninitializedTestFileBuffer(t, ctrl)

		mock.EXPECT().Close().Return(nil).Times(1)
		assert.NoError(t, f.Close())
		assert.NoError(t, f.init(defaultFileName, cell.NewBuffer(), "", false))
	})

	t.Run("two consecutive calls to Close should return an error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		f, mock, _ := newUninitializedTestFileBuffer(t, ctrl)

		mock.EXPECT().Close().Return(nil).Times(1)
		assert.NoError(t, f.Close())
		assert.Error(t, f.Close())
	})
}

func testFileBufferFlush(t *testing.T, newBuffer newBufferFunc) {
	t.Run("Flush bubbles up Stat errors", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		f, _, _ := newBuffer(t, ctrl)

		myErr := errors.New("what?")
		f.scheme.(*testScheme).statFunc = func(name string) (os.FileInfo, error) {
			return nil, myErr
		}
		assert.Equal(t, myErr, f.Flush())
	})

	t.Run("flush syncs the contents of the buffer to disk", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		f, mock, _ := newBuffer(t, ctrl)
		called := false
		f.scheme.(*testScheme).renameFunc = func(oldName, newName string) error {
			called = true
			return nil
		}

		mock.EXPECT().Close().Return(nil).Times(2)
		expectInitSwap(mock, defaultFileName, testFileInfo{}, defaultFileData)

		assert.NoError(t, f.Flush())
		assert.True(t, called)
	})

	t.Run("flush bubbles up rename errors", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		f, mock, _ := newBuffer(t, ctrl)
		myErr := errors.New("wtf")
		f.scheme.(*testScheme).renameFunc = func(oldName, newName string) error {
			return myErr
		}

		mock.EXPECT().Close().Return(nil).Times(2)
		assert.Equal(t, myErr, f.Flush())
	})

	t.Run("flush returns error if file was modified", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		f, _, _ := newBuffer(t, ctrl)
		f.scheme.(*testScheme).statFunc = func(name string) (os.FileInfo, error) {
			return testFileInfo{modTime: time.Now()}, nil
		}
		assert.Error(t, workspaceapi.ErrStaleData, f.Flush())
	})
}

func TestNewFileBufferFlush(t *testing.T) {
	testFileBufferFlush(t, newInitializedTestFileBuffer)

	t.Run("Flush returns ErrFileIsNotWritable if file was opened in read-only", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		f, _, _ := newReadOnlyTestFileBuffer(t, ctrl)
		assert.Equal(t, workspaceapi.ErrFileIsNotWritable, f.Flush())
	})

	t.Run("if file is created after NewFileBuffer is called returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		f, _, _ := newUninitializedTestFileBuffer(t, ctrl)
		require.Nil(t, f.orig)

		f.scheme.(*testScheme).openFunc = func(name string, flag int, perm os.FileMode) (workspaceapi.File, error) {
			assert.NotZero(t, flag&os.O_CREATE)
			return nil, os.ErrExist
		}
		assert.Equal(t, workspaceapi.ErrStaleData, f.Flush())
	})
}

func TestRecoverFileBufferFlush(t *testing.T) {
	testFileBufferFlush(t, newRecoveredTestFileBuffer)
}

func expectCopyToSwapPrepare(f *file, mock *workspaceapitest.MockFile) func() {
	mock.EXPECT().Truncate(gomock.Eq(int64(0))).Return(nil)
	mock.EXPECT().Seek(gomock.Eq(int64(0)), gomock.Eq(0)).Return(int64(0), nil)
	return f.wg.Wait
}

func expectCopyToSwap(f *file, mock *workspaceapitest.MockFile, newData string) func() {
	expectedContent := newData + string(defaultFileData)
	clean := expectCopyToSwapPrepare(f, mock)
	mock.EXPECT().
		Write(gomock.Eq([]byte(expectedContent+"\n"))).
		Return(len(expectedContent)+1, nil)
	mock.EXPECT().Sync().Return(nil)
	mock.EXPECT().Stat().Return(testFileInfo{}, nil).AnyTimes()
	return clean
}

type newBufferFunc func(*testing.T, *gomock.Controller) (*file, *workspaceapitest.MockFile, *cell.Buffer)

// TODO debug why it's failing sometimes
func testFileBufferInsert(
	t *testing.T,
	newBuffer newBufferFunc,
) {

	t.Run("if copy to swap fails, retries on next insert", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		f, mock, buf := newBuffer(t, ctrl)
		myString := "my string\n"
		myError := errors.New("I feel clammy")

		mock.EXPECT().Truncate(gomock.Eq(int64(0))).Return(myError)
		buf.InsertString(term.Coordinates{}, myString)
		f.wg.Wait()

		wait := expectCopyToSwap(f, mock, myString+myString)
		defer wait()

		buf.InsertString(term.Coordinates{}, myString)
	})

	t.Run("if copy to swap fails, retries on next Flush", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		f, mock, buf := newBuffer(t, ctrl)
		myString := "Sant Hipòlit de Voltregà"
		myError := errors.New("No té capità")

		mock.EXPECT().Truncate(gomock.Eq(int64(0))).Return(nil)
		mock.EXPECT().Seek(gomock.Eq(int64(0)), gomock.Eq(0)).Return(int64(0), myError)
		buf.InsertString(term.Coordinates{}, myString)
		f.wg.Wait()

		wait := expectCopyToSwap(f, mock, myString)
		defer wait()

		mock.EXPECT().Close().Return(nil).Times(2)
		expectInitSwap(mock, defaultFileName, testFileInfo{}, defaultFileData)
		assert.NoError(t, f.Flush())
	})

	t.Run("if copy to swap fails, retries on next Flush and fails, bubbles up error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		f, mock, buf := newBuffer(t, ctrl)
		myString := "Ballz"
		myError := errors.New("rounder")

		mock.EXPECT().Truncate(gomock.Eq(int64(0))).Return(myError)
		buf.InsertString(term.Coordinates{}, myString)
		f.wg.Wait()

		mock.EXPECT().Truncate(gomock.Eq(int64(0))).Return(myError)
		assert.Error(t, f.Flush())
	})

	t.Run("if copy to swap fails, errors contains details of swap file", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		f, mock, buf := newBuffer(t, ctrl)
		myString := "a"
		myError := errors.New("access super-denied")

		mock.EXPECT().Truncate(gomock.Eq(int64(0))).Return(myError)
		buf.InsertString(term.Coordinates{}, myString)
		f.wg.Wait()

		mock.EXPECT().Truncate(gomock.Eq(int64(0))).Return(myError)
		err := f.Flush()
		require.Error(t, err)
		assert.Contains(t, err.Error(), defaultFileName)
	})

	t.Run("Undo/Redo should be captured and therefore copied to swap file", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		f, mock, buf := newBuffer(t, ctrl)
		myString := "my string\n"

		// wait for all of them otherwise if test goroutine is slow
		// file could chose to skip one of the syncs
		wait := expectCopyToSwap(f, mock, myString)
		buf.InsertString(term.Coordinates{}, myString)
		wait()

		wait = expectCopyToSwap(f, mock, "")
		buf.Undo()
		wait()

		wait = expectCopyToSwap(f, mock, myString)
		buf.Redo()
		wait()
	})

	t.Run("upon Insert, it copies content to swap file", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		f, mock, buf := newBuffer(t, ctrl)
		require.Equal(t, string(defaultFileData), buf.String())

		myString := "my string\n"
		wait := expectCopyToSwap(f, mock, myString)
		defer wait()

		buf.InsertString(term.Coordinates{}, myString)
	})

	t.Run("upon Insert, if file is not writeable, it does nothing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		fbuf, _, buf := newBuffer(t, ctrl)
		fbuf.swap = nil
		buf.InsertString(term.Coordinates{}, "blah")
	})
}

func testFileBufferDelete(t *testing.T, newBuffer newBufferFunc) {
	t.Run("upon Delete, it copies content to swap file", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		f, mock, buf := newBuffer(t, ctrl)
		wait := expectCopyToSwapPrepare(f, mock)
		defer wait()

		mock.EXPECT().
			Write(gomock.Eq([]byte("\n"))).
			Return(1, nil)
		mock.EXPECT().Sync().Return(nil)
		buf.DeleteRow(0)
	})

	t.Run("upon Delete, if file is not writeable, it does nothing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		fbuf, _, buf := newBuffer(t, ctrl)
		fbuf.swap = nil
		buf.DeleteRow(0)
	})
}

func TestNewFileBufferDelete(t *testing.T) {
	testFileBufferDelete(t, newInitializedTestFileBuffer)
}

func TestNewFileBufferInsert(t *testing.T) {
	testFileBufferInsert(t, newInitializedTestFileBuffer)
}

func TestRecoverFileBufferDelete(t *testing.T) {
	testFileBufferDelete(t, newRecoveredTestFileBuffer)
}

func TestRecoverFileBufferInsert(t *testing.T) {
	testFileBufferInsert(t, newRecoveredTestFileBuffer)
}

func TestFileMissingLastCopySwap(t *testing.T) {
	buf, file := newIntegrationTestCase(t, true)

	f, err := openFile(file.Name(), buf, "", false)
	require.NoError(t, err)
	defer f.Close()

	var builder strings.Builder
	builder.Write([]byte(sampleSnippet))

	for i := 0; i < 1000; i++ {
		buf.WriteString("a")
		builder.WriteString("a")
	}
	require.NoError(t, f.Flush())

	builder.Write([]byte("\n"))
	want := builder.String()
	actual, err := os.ReadFile(file.Name())
	require.NoError(t, f.Flush())
	assert.Equal(t, want, string(actual))
}
