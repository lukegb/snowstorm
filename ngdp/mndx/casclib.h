#ifdef __cplusplus
extern "C" {
#endif

#include <stdint.h>
#include <stdlib.h>

#ifndef MD5_HASH_SIZE
#define MD5_HASH_SIZE 0x10
#endif

#ifndef MAX_PATH
#define MAX_PATH 1024
#endif


extern void add_file(uint32_t num, char* filename, uint32_t fileSize, uint32_t localeFlags, uint32_t fileDataID, void* encKey, int encKeyLen);

struct mndx_file {
	char* name;
	uint32_t size;
	uint32_t localeFlags;
	uint32_t fileDataID;
	uint8_t encodingKey[MD5_HASH_SIZE];
};

int DoTheThing(void* pbRootFile, uint32_t cbRootFile, struct mndx_file** files, uint32_t* fileCount);
void FreeTheThings(struct mndx_file* files, uint32_t fileCount);

#ifdef __cplusplus
}
#endif
