/*
Copyright 2017 Luke Granger-Brown

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/


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
