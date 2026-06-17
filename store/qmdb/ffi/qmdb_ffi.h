#ifndef QMDB_FFI_H
#define QMDB_FFI_H

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

// Opaque handles
typedef struct QmdbHandle QmdbHandle;
typedef struct ChangeSetHandle ChangeSetHandle;

typedef struct {
    uint32_t size;
    uint8_t found;  // 1 = found, 0 = not found
} QmdbReadResult;

// Lifecycle
int32_t qmdb_init(const char *dir);
QmdbHandle *qmdb_open(const char *dir);
void qmdb_close(QmdbHandle *handle);
int64_t qmdb_height(const QmdbHandle *handle);

// Read
QmdbReadResult qmdb_get(
    const QmdbHandle *handle,
    const uint8_t *key_ptr, uint32_t key_len,
    uint8_t *val_buf, uint32_t val_buf_len
);

// Changeset building
ChangeSetHandle *qmdb_changeset_new(void);
void qmdb_changeset_create(ChangeSetHandle *cs,
    const uint8_t *key_ptr, uint32_t key_len,
    const uint8_t *val_ptr, uint32_t val_len);
void qmdb_changeset_write(ChangeSetHandle *cs,
    const uint8_t *key_ptr, uint32_t key_len,
    const uint8_t *val_ptr, uint32_t val_len);
void qmdb_changeset_delete(ChangeSetHandle *cs,
    const uint8_t *key_ptr, uint32_t key_len);
void qmdb_changeset_free(ChangeSetHandle *cs);

// Commit
int64_t qmdb_commit(QmdbHandle *handle, ChangeSetHandle *cs);

// Hash
int32_t qmdb_root_hash(const QmdbHandle *handle, int64_t height, uint8_t *out_buf);

// Height override (used by migration)
int32_t qmdb_set_height(QmdbHandle *handle, int64_t height);

#ifdef __cplusplus
}
#endif

#endif // QMDB_FFI_H
