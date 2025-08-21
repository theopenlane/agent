package storage

import "errors"

var (
	// ErrStorageNotInitialized is returned when storage is not initialized
	ErrStorageNotInitialized = errors.New("storage not initialized")
	// ErrInvalidStorageMode is returned when storage mode is invalid
	ErrInvalidStorageMode = errors.New("invalid storage mode")
	// ErrFailedToCreateDirectory is returned when directory creation fails
	ErrFailedToCreateDirectory = errors.New("failed to create directory")
	// ErrFailedToWriteFile is returned when file writing fails
	ErrFailedToWriteFile = errors.New("failed to write file")
	// ErrFailedToReadFile is returned when file reading fails
	ErrFailedToReadFile = errors.New("failed to read file")
	// ErrFailedToMarshalData is returned when data marshaling fails
	ErrFailedToMarshalData = errors.New("failed to marshal data")
	// ErrFailedToUnmarshalData is returned when data unmarshaling fails
	ErrFailedToUnmarshalData = errors.New("failed to unmarshal data")
	// ErrFileNotFound is returned when file is not found
	ErrFileNotFound = errors.New("file not found")
	// ErrDirectoryNotFound is returned when directory is not found
	ErrDirectoryNotFound = errors.New("directory not found")
	// ErrFailedToCalculateChecksum is returned when checksum calculation fails
	ErrFailedToCalculateChecksum = errors.New("failed to calculate checksum")
	// ErrFailedToCompressFile is returned when file compression fails
	ErrFailedToCompressFile = errors.New("failed to compress file")
	// ErrInvalidPath is returned when path is invalid
	ErrInvalidPath = errors.New("invalid path")
	// ErrPathOutsideDataDir is returned when path is outside data directory
	ErrPathOutsideDataDir = errors.New("path outside data directory")
	// ErrAPIURLRequired is returned when API URL is required for API storage
	ErrAPIURLRequired = errors.New("API URL is required for API storage")
	// ErrRegistrationTokenRequired is returned when registration token is required for API storage
	ErrRegistrationTokenRequired = errors.New("registration token is required for API storage")
)
