package store

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings" // For sorting by path
	"time"

	"github.com/kanon1343/fsegit/sha"
	// "github.com/kanon1343/fsegit/util" // Not strictly needed for these funcs if gitDir not used directly
)

const (
	indexHeaderSignature = "DIRC"
	indexVersion         = 2
	indexHeaderSize      = 12 // 4 (sig) + 4 (ver) + 4 (num_entries)
	// Max path length stored in the 12 LSB of the flags field
	maxPathLength = 0xFFF 
)

// IndexEntry represents a single entry in the Git index file.
type IndexEntry struct {
	CTimeSeconds      uint32
	CTimeNanoseconds  uint32
	MTimeSeconds      uint32
	MTimeNanoseconds  uint32
	Dev               uint32
	Ino               uint32
	Mode              uint32 
	UID               uint32
	GID               uint32
	Size              uint32 
	Hash              sha.SHA1 
	PathName          string
	Flags             uint16 // Contains path length (lower 12 bits) and stage information (next 2 bits). Public field.
}

// SetPackedFlags sets the 16-bit flags field for an index entry.
// It combines pathLength (up to 0xFFF) and stage (2 bits: 0-3).
func (e *IndexEntry) SetPackedFlags(stage uint8, pathLength int) {
	if pathLength > maxPathLength {
		pathLength = maxPathLength
	}
	// Stage bits are bits 12 and 13 (0-indexed).
	// Git uses stage 0 for normal, 1 for base, 2 for ours, 3 for theirs.
	e.Flags = (uint16(stage&0x3) << 12) | (uint16(pathLength) & maxPathLength)
}

// Index represents the entire Git index (staging area).
type Index struct {
	Version  uint32
	Entries  []*IndexEntry
	filePath string 
}

// newIndex creates a new, empty Index object.
func newIndex(filePath string) *Index {
	return &Index{
		Version:  indexVersion,
		Entries:  make([]*IndexEntry, 0),
		filePath: filePath,
	}
}

// ReadIndex reads and parses the .git/index file from the given Git directory.
// (Assumed to be already implemented correctly from previous step)
func ReadIndex(gitDir string) (*Index, error) {
	indexPath := filepath.Join(gitDir, "index")
	idx := newIndex(indexPath)

	data, err := ioutil.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return idx, nil 
		}
		return nil, fmt.Errorf("failed to read index file %s: %w", indexPath, err)
	}

	if len(data) < indexHeaderSize {
		// If data is present but less than header, it's likely corrupt or not a git index.
		return nil, fmt.Errorf("invalid index file %s: too short for header", indexPath)
	}
	
	// Verify Checksum first (if file is long enough)
	if len(data) >= indexHeaderSize+sha.HashSize { // Check if data is long enough for header AND checksum
		contentToCheck := data[:len(data)-sha.HashSize]
		expectedChecksum := data[len(data)-sha.HashSize:]
		actualChecksum := sha1.Sum(contentToCheck)
		if !bytes.Equal(expectedChecksum, actualChecksum[:]) {
			return nil, fmt.Errorf("invalid index file %s: checksum mismatch", indexPath)
		}
		// Trim data to not include checksum for parsing entries
		data = contentToCheck
	} else if len(data) > indexHeaderSize && len(data) < indexHeaderSize+sha.HashSize {
        // File has a header, maybe some entry data, but not enough for a full checksum
        return nil, fmt.Errorf("invalid index file %s: too short for checksum but has header", indexPath)
    } else if len(data) == indexHeaderSize && len(data) < indexHeaderSize+sha.HashSize {
		// File is exactly header size, no entries, no checksum. This is valid for an empty index that was written.
		// However, if it was written by this WriteIndex, it should have a checksum.
		// For robustness, let's consider an index with 0 entries.
		// Its on-disk representation would be: Header (12 bytes) + Checksum (20 bytes) = 32 bytes.
		// If len(data) is only 12, it means it's an empty index without a checksum, which can happen
		// if it's newly created by 'git init' or similar minimal states before any entries are added.
		// The original ReadIndex would return an empty idx for a non-existent file.
		// If a file exists and is only 12 bytes, it's an empty index, no entries to parse.
		// The numEntries will be 0. Loop for entries won't run.
		// The final check `offset != len(data)` will pass because offset will be 12 and len(data) will be 12.
	}


	header := data[:indexHeaderSize]
	signature := string(header[0:4])
	if signature != indexHeaderSignature {
		return nil, fmt.Errorf("invalid index file %s: bad signature %q", indexPath, signature)
	}

	idx.Version = binary.BigEndian.Uint32(header[4:8])
	if idx.Version != indexVersion {
		return nil, fmt.Errorf("unsupported index version %d in %s", idx.Version, indexPath)
	}
	numEntries := binary.BigEndian.Uint32(header[8:12])

	offset := indexHeaderSize
	idx.Entries = make([]*IndexEntry, 0, numEntries)

	for i := 0; i < int(numEntries); i++ {
		if offset >= len(data) { 
			return nil, fmt.Errorf("index file %s: insufficient data for entry %d, expected %d entries. Offset: %d, Data Length: %d", indexPath, i, numEntries, offset, len(data))
		}
		entry := &IndexEntry{}
		entryStartOffset := offset 

		fieldsSizeWithoutPathAndPadding := 60 + 2 // CTime to Size (60 bytes) + flags (2 bytes)
		if offset+fieldsSizeWithoutPathAndPadding > len(data) { 
			return nil, fmt.Errorf("index file %s: insufficient data for fixed fields of entry %d", indexPath, i)
		}

		entry.CTimeSeconds = binary.BigEndian.Uint32(data[offset : offset+4]); offset += 4
		entry.CTimeNanoseconds = binary.BigEndian.Uint32(data[offset : offset+4]); offset += 4
		entry.MTimeSeconds = binary.BigEndian.Uint32(data[offset : offset+4]); offset += 4
		entry.MTimeNanoseconds = binary.BigEndian.Uint32(data[offset : offset+4]); offset += 4
		entry.Dev = binary.BigEndian.Uint32(data[offset : offset+4]); offset += 4
		entry.Ino = binary.BigEndian.Uint32(data[offset : offset+4]); offset += 4
		entry.Mode = binary.BigEndian.Uint32(data[offset : offset+4]); offset += 4
		entry.UID = binary.BigEndian.Uint32(data[offset : offset+4]); offset += 4
		entry.GID = binary.BigEndian.Uint32(data[offset : offset+4]); offset += 4
		entry.Size = binary.BigEndian.Uint32(data[offset : offset+4]); offset += 4

		entry.Hash = make(sha.SHA1, sha.HashSize)
		copy(entry.Hash, data[offset:offset+sha.HashSize]); offset += sha.HashSize

		entry.Flags = binary.BigEndian.Uint16(data[offset : offset+2]); offset += 2
		
		pathLen := int(entry.Flags & maxPathLength)

		if offset+pathLen > len(data) {
			return nil, fmt.Errorf("index file %s: insufficient data for path name of entry %d (pathLen %d)", indexPath, i, pathLen)
		}
		entry.PathName = string(data[offset : offset+pathLen]); offset += pathLen
		
		entryActualDiskLength := (offset - entryStartOffset) 
		padding := (8 - (entryActualDiskLength % 8)) % 8
		
		if offset+padding > len(data) {
			return nil, fmt.Errorf("index file %s: insufficient data for padding of entry %d", indexPath, i)
		}
		offset += padding 

		idx.Entries = append(idx.Entries, entry)
	}
    if offset != len(data) { 
        return nil, fmt.Errorf("index file %s: data corruption, offset %d does not match data length %d after parsing %d entries", indexPath, offset, len(data), numEntries)
    }
	return idx, nil
}


// WriteIndex writes the current Index object to its filePath.
func WriteIndex(idx *Index) error {
	// Sort entries by path name (and stage, if applicable, though stage is not fully handled here)
	sort.Slice(idx.Entries, func(i, j int) bool {
		// Basic sort by path name. For full Git compatibility, stage should also be considered.
		return idx.Entries[i].PathName < idx.Entries[j].PathName
	})

	var buffer bytes.Buffer

	// Write Header
	if err := binary.Write(&buffer, binary.BigEndian, []byte(indexHeaderSignature)); err != nil {
		return err
	}
	if err := binary.Write(&buffer, binary.BigEndian, idx.Version); err != nil {
		return err
	}
	if err := binary.Write(&buffer, binary.BigEndian, uint32(len(idx.Entries))); err != nil {
		return err
	}

	// Write Entries
	for _, entry := range idx.Entries {
		if err := binary.Write(&buffer, binary.BigEndian, entry.CTimeSeconds); err != nil { return err }
		if err := binary.Write(&buffer, binary.BigEndian, entry.CTimeNanoseconds); err != nil { return err }
		if err := binary.Write(&buffer, binary.BigEndian, entry.MTimeSeconds); err != nil { return err }
		if err := binary.Write(&buffer, binary.BigEndian, entry.MTimeNanoseconds); err != nil { return err }
		if err := binary.Write(&buffer, binary.BigEndian, entry.Dev); err != nil { return err }
		if err := binary.Write(&buffer, binary.BigEndian, entry.Ino); err != nil { return err }
		if err := binary.Write(&buffer, binary.BigEndian, entry.Mode); err != nil { return err }
		if err := binary.Write(&buffer, binary.BigEndian, entry.UID); err != nil { return err }
		if err := binary.Write(&buffer, binary.BigEndian, entry.GID); err != nil { return err }
		if err := binary.Write(&buffer, binary.BigEndian, entry.Size); err != nil { return err }
		
		// Hash is already sha.SHA1 ([]byte), directly write it.
		if _, err := buffer.Write(entry.Hash); err != nil { return err }
		
		// Ensure path length does not exceed maxPathLength
		pathLen := len(entry.PathName)
		if pathLen > maxPathLength {
			return fmt.Errorf("path name %q is too long (%d bytes, max %d)", entry.PathName, pathLen, maxPathLength)
		}
		// For simplicity, flags field only contains path length. Stage bits are zero.
		// Write the pre-computed Flags field.
		// The caller (e.g., add command) is responsible for setting this correctly using SetPackedFlags.
		if err := binary.Write(&buffer, binary.BigEndian, entry.Flags); err != nil { return err }
		
		// Write PathName as raw bytes
		if err := binary.Write(&buffer, binary.BigEndian, []byte(entry.PathName)); err != nil { return err }

		// Calculate padding
		// Length of entry from CTimeSeconds to end of PathName string (as written to disk)
		// 10 * 4 (fixed fields CTime to Size) + 20 (hash) + 2 (Flags) + len(entry.PathName)
		entryCoreLengthOnDisk := 60 + 2 + len(entry.PathName)
		paddingSize := (8 - (entryCoreLengthOnDisk % 8)) % 8
		if paddingSize > 0 {
			paddingBytes := make([]byte, paddingSize) // Zero bytes
			if _, err := buffer.Write(paddingBytes); err != nil { return err }
		}
	}

	// Calculate and append checksum
	checksum := sha1.Sum(buffer.Bytes())
	if _, err := buffer.Write(checksum[:]); err != nil {
		return err
	}

	// Write buffer to file
	return ioutil.WriteFile(idx.filePath, buffer.Bytes(), 0644) // Standard file permissions
}


// AddEntry adds or replaces an entry in the index.
func (idx *Index) AddEntry(newEntry *IndexEntry) {
	for i, entry := range idx.Entries {
		if entry.PathName == newEntry.PathName {
			idx.Entries[i] = newEntry // Replace existing
			return
		}
	}
	idx.Entries = append(idx.Entries, newEntry) // Add new
    // Note: Sorting is handled by WriteIndex before writing.
}

// RemoveEntry removes an entry by its path name. Returns true if removed.
func (idx *Index) RemoveEntry(pathName string) bool {
	for i, entry := range idx.Entries {
		if entry.PathName == pathName {
			idx.Entries = append(idx.Entries[:i], idx.Entries[i+1:]...)
			return true
		}
	}
	return false
}

// GetEntryByName retrieves an entry by its path name.
func (idx *Index) GetEntryByName(pathName string) *IndexEntry {
	for _, entry := range idx.Entries {
		if entry.PathName == pathName {
			return entry
		}
	}
	return nil
}

// The TODO for helper functions was removed in a previous step as they are implemented.
// The primary TODO remaining would be for handling index extensions if any.
// For now, the core functionality is covered.
// A function like `FindGitDir()` or similar would be needed to make ReadIndex truly standalone
// if not provided with an explicit gitDir. For now, gitDir is a parameter.
// The util import was commented out as it's not used in this specific provided code block.
// If FindGitRoot or similar utils are used elsewhere with Index, it should be uncommented.
