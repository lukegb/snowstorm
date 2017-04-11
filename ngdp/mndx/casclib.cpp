#include <stdint.h>
#include <stdlib.h>

#include <iostream>

#ifdef WHO_NEEDS_A_BUILD_SYSTEM_ANYWAY
#include "CascRootFile_Mndx.cpp"
#include "common/FileStream.cpp"
#include "common/DumpContext.cpp"
#include "common/RootHandler.cpp"
#include "common/Common.cpp"
#include "jenkins/lookup3.c"
#include "libtomcrypt/src/hashes/md5.c"
#include "libtomcrypt/src/misc/crypt_argchk.c"
#endif

#include "CascLib.h"
#include "CascCommon.h"
#include "CascMndx.h"

#include "casclib.h"

#ifndef WHO_NEEDS_A_BUILD_SYSTEM_ANYWAY
typedef struct _CASC_MNDX_INFO
{
    BYTE  RootFileName[MD5_HASH_SIZE];              // Name (aka MD5) of the root file
    DWORD HeaderVersion;                            // Must be <= 2
    DWORD FormatVersion;
    DWORD field_1C;
    DWORD field_20;
    DWORD MarInfoOffset;                            // Offset of the first MAR entry info
    DWORD MarInfoCount;                             // Number of the MAR info entries
    DWORD MarInfoSize;                              // Size of the MAR info entry
    DWORD MndxEntriesOffset;
    DWORD MndxEntriesTotal;                         // Total number of MNDX root entries
    DWORD MndxEntriesValid;                         // Number of valid MNDX root entries
    DWORD MndxEntrySize;                            // Size of one MNDX root entry
    struct _MAR_FILE * pMarFile1;                   // File name list for the packages
    struct _MAR_FILE * pMarFile2;                   // File name list for names stripped of package names
    struct _MAR_FILE * pMarFile3;                   // File name list for complete names
//  PCASC_ROOT_ENTRY_MNDX pMndxEntries;
//  PCASC_ROOT_ENTRY_MNDX * ppValidEntries;
    bool bRootFileLoaded;                           // true if the root info file was properly loaded

} CASC_MNDX_INFO, *PCASC_MNDX_INFO;

struct TRootHandler_MNDX : public TRootHandler
{
    CASC_MNDX_INFO MndxInfo;
};
#endif

void FreeTheThings(struct mndx_file* files, uint32_t fileCount) {
	for (uint32_t i = 0; i < fileCount; i++) {
		struct mndx_file* f = files + i;
		free(f->name);
	}
	free(files);
}

int DoTheThing(void* pbRootFile, uint32_t cbRootFile, struct mndx_file** files, uint32_t* fileCount) {
	TCascStorage* hs = new TCascStorage;
	int rc = RootHandler_CreateMNDX(hs, (LPBYTE)pbRootFile, cbRootFile);
	if (rc != ERROR_SUCCESS) {
		return rc;
	}

	// Calculate the total number of files
	DWORD fileNameCount = 0;
	TRootHandler_MNDX* mndxHandler = static_cast<TRootHandler_MNDX*>(hs->pRootHandler);
	mndxHandler->MndxInfo.pMarFile3->pDatabasePtr->GetFileNameCount(&fileNameCount);

	// Allocate output buffer
	struct mndx_file* outBuf = static_cast<struct mndx_file*>(calloc(fileNameCount, sizeof(struct mndx_file)));

	TCascSearch *pSearch = (TCascSearch*)calloc(1, sizeof(TCascSearch) + 1024);
	pSearch->szClassName = "TCascSearch";
	pSearch->hs = hs;
	pSearch->szMask = CascNewStr("*", 0);

	LPBYTE pbEncodingKey;
	uint32_t i = 0;
	while (true) {
		struct mndx_file* f = outBuf + i;

		pbEncodingKey = RootHandler_Search(pSearch->hs->pRootHandler, pSearch, &f->size, &f->localeFlags, &f->fileDataID);
		if (pbEncodingKey == NULL) break;

		f->name = static_cast<char*>(malloc(strlen(pSearch->szFileName)+1));
		strcpy(f->name, pSearch->szFileName);

		memcpy(&f->encodingKey, pbEncodingKey, MD5_HASH_SIZE);

		++i;
	}
	*fileCount = fileNameCount;
	*files = outBuf;

	RootHandler_EndSearch(pSearch->hs->pRootHandler, pSearch);
	CASC_FREE(pSearch->szMask);
	pSearch->szClassName = NULL;
	free(pSearch);

	RootHandler_Close(hs->pRootHandler);
	delete hs;

	return rc;
}
