use std::ffi::CStr;
use std::os::raw::c_char;
use std::ptr;
use std::sync::Arc;

use qmdb::config::Config;
use qmdb::def::{IN_BLOCK_IDX_BITS, OP_CREATE, OP_DELETE, OP_WRITE};
use qmdb::entryfile::EntryBz;
use qmdb::seqads::SeqAdsWrap;
use qmdb::tasks::helpers::SimpleTask;
use qmdb::utils::changeset::ChangeSet;
use qmdb::utils::{byte0_to_shard_id, hasher};
use qmdb::AdsCore;
use qmdb::ADS;

// Opaque handle to the database
pub struct QmdbHandle {
    ads: SeqAdsWrap<SimpleTask>,
    height: i64,
}

// Opaque handle to a changeset being built
pub struct ChangeSetHandle {
    change_set: ChangeSet,
}

/// Result of a read operation.
#[repr(C)]
pub struct QmdbReadResult {
    pub size: u32,
    pub found: u8,
}

/// Initialize a new qmdb database at the given directory.
#[no_mangle]
pub unsafe extern "C" fn qmdb_init(dir: *const c_char) -> i32 {
    if dir.is_null() {
        return -1;
    }
    let dir_str = match CStr::from_ptr(dir).to_str() {
        Ok(s) => s,
        Err(_) => return -1,
    };
    let config = Config {
        dir: dir_str.to_string(),
        with_twig_file: true,
        ..Config::default()
    };
    AdsCore::init_dir(&config);
    0
}

/// Open an existing qmdb database. Returns null on failure.
#[no_mangle]
pub unsafe extern "C" fn qmdb_open(dir: *const c_char) -> *mut QmdbHandle {
    if dir.is_null() {
        return ptr::null_mut();
    }
    let dir_str = match CStr::from_ptr(dir).to_str() {
        Ok(s) => s,
        Err(_) => return ptr::null_mut(),
    };
    let config = Config {
        dir: dir_str.to_string(),
        with_twig_file: true,
        ..Config::default()
    };

    let ads = SeqAdsWrap::<SimpleTask>::new(&config);
    let height = ads.get_metadb().read().unwrap().get_curr_height();

    Box::into_raw(Box::new(QmdbHandle { ads, height }))
}

/// Close and free a qmdb handle.
#[no_mangle]
pub unsafe extern "C" fn qmdb_close(handle: *mut QmdbHandle) {
    if !handle.is_null() {
        drop(Box::from_raw(handle));
    }
}

/// Get the current block height.
#[no_mangle]
pub unsafe extern "C" fn qmdb_height(handle: *const QmdbHandle) -> i64 {
    if handle.is_null() {
        return -1;
    }
    (*handle).height
}

/// Read a value by key. Writes value bytes into val_buf.
#[no_mangle]
pub unsafe extern "C" fn qmdb_get(
    handle: *const QmdbHandle,
    key_ptr: *const u8,
    key_len: u32,
    val_buf: *mut u8,
    val_buf_len: u32,
) -> QmdbReadResult {
    if handle.is_null() || key_ptr.is_null() {
        return QmdbReadResult { size: 0, found: 0 };
    }

    let h = &*handle;
    let key = std::slice::from_raw_parts(key_ptr, key_len as usize);
    let key_hash = hasher::hash(key);

    // Buffer must hold the full entry (key + value + metadata ~80 bytes overhead)
    let buf_size = std::cmp::max(val_buf_len as usize + 256, 65536);
    let mut buf = vec![0u8; buf_size];
    let (size, found) = h.ads.read_entry(h.height, &key_hash, key, &mut buf);

    if !found || size == 0 {
        return QmdbReadResult { size: 0, found: 0 };
    }

    // Parse entry format to extract value
    let entry_bz = EntryBz { bz: &buf[..size] };
    let value = entry_bz.value();

    let result_size = value.len() as u32;
    if !val_buf.is_null() && val_buf_len >= result_size {
        std::ptr::copy_nonoverlapping(value.as_ptr(), val_buf, value.len());
    }

    QmdbReadResult {
        size: result_size,
        found: 1,
    }
}

/// Create a new changeset for batching writes.
#[no_mangle]
pub extern "C" fn qmdb_changeset_new() -> *mut ChangeSetHandle {
    Box::into_raw(Box::new(ChangeSetHandle {
        change_set: ChangeSet::new(),
    }))
}

/// Add a CREATE operation to the changeset.
#[no_mangle]
pub unsafe extern "C" fn qmdb_changeset_create(
    cs: *mut ChangeSetHandle,
    key_ptr: *const u8,
    key_len: u32,
    val_ptr: *const u8,
    val_len: u32,
) {
    if cs.is_null() || key_ptr.is_null() {
        return;
    }
    let cs = &mut *cs;
    let key = std::slice::from_raw_parts(key_ptr, key_len as usize);
    let val = if val_ptr.is_null() {
        &[]
    } else {
        std::slice::from_raw_parts(val_ptr, val_len as usize)
    };
    let key_hash = hasher::hash(key);
    let shard_id = byte0_to_shard_id(key_hash[0]) as u8;
    cs.change_set
        .add_op(OP_CREATE, shard_id, &key_hash, key, val, None);
}

/// Add a WRITE (update) operation to the changeset.
#[no_mangle]
pub unsafe extern "C" fn qmdb_changeset_write(
    cs: *mut ChangeSetHandle,
    key_ptr: *const u8,
    key_len: u32,
    val_ptr: *const u8,
    val_len: u32,
) {
    if cs.is_null() || key_ptr.is_null() {
        return;
    }
    let cs = &mut *cs;
    let key = std::slice::from_raw_parts(key_ptr, key_len as usize);
    let val = if val_ptr.is_null() {
        &[]
    } else {
        std::slice::from_raw_parts(val_ptr, val_len as usize)
    };
    let key_hash = hasher::hash(key);
    let shard_id = byte0_to_shard_id(key_hash[0]) as u8;
    cs.change_set
        .add_op(OP_WRITE, shard_id, &key_hash, key, val, None);
}

/// Add a DELETE operation to the changeset.
#[no_mangle]
pub unsafe extern "C" fn qmdb_changeset_delete(
    cs: *mut ChangeSetHandle,
    key_ptr: *const u8,
    key_len: u32,
) {
    if cs.is_null() || key_ptr.is_null() {
        return;
    }
    let cs = &mut *cs;
    let key = std::slice::from_raw_parts(key_ptr, key_len as usize);
    let key_hash = hasher::hash(key);
    let shard_id = byte0_to_shard_id(key_hash[0]) as u8;
    cs.change_set
        .add_op(OP_DELETE, shard_id, &key_hash, key, &[], None);
}

/// Free a changeset without committing.
#[no_mangle]
pub unsafe extern "C" fn qmdb_changeset_free(cs: *mut ChangeSetHandle) {
    if !cs.is_null() {
        drop(Box::from_raw(cs));
    }
}

/// Commit a changeset as a new block. Consumes the changeset.
/// Returns the new block height, or -1 on error.
#[no_mangle]
pub unsafe extern "C" fn qmdb_commit(
    handle: *mut QmdbHandle,
    cs: *mut ChangeSetHandle,
) -> i64 {
    if handle.is_null() || cs.is_null() {
        return -1;
    }

    let h = &mut *handle;
    let mut cs = Box::from_raw(cs);

    // Sort the changeset (required by qmdb)
    cs.change_set.sort();

    let new_height = h.height + 1;

    // task_id encodes height in upper bits
    let task_id = (new_height << IN_BLOCK_IDX_BITS) as i64;

    // Build the change_sets vec
    let change_sets = vec![cs.change_set];

    // Insert extra data (required by qmdb before commit)
    h.ads
        .get_metadb()
        .write()
        .unwrap()
        .insert_extra_data(new_height, String::new());

    // Commit the transaction
    h.ads.commit_tx(task_id, &change_sets);

    // Finalize block
    h.ads.commit_block(new_height);

    h.height = new_height;
    new_height
}

/// Override the current height. Used after migration so the next commit
/// produces the correct block height (halt_height + 1).
///
/// Only sets the in-memory height. The metadb gets updated on the next
/// commit_block call, so don't crash between set_height and the first commit.
#[no_mangle]
pub unsafe extern "C" fn qmdb_set_height(handle: *mut QmdbHandle, height: i64) -> i32 {
    if handle.is_null() {
        return -1;
    }
    (*handle).height = height;
    0
}

/// Get the root hash at a given height. Writes 32 bytes to out_buf.
#[no_mangle]
pub unsafe extern "C" fn qmdb_root_hash(
    handle: *const QmdbHandle,
    height: i64,
    out_buf: *mut u8,
) -> i32 {
    if handle.is_null() || out_buf.is_null() {
        return -1;
    }

    let h = &*handle;
    let hash = h.ads.get_root_hash_of_height(height);
    std::ptr::copy_nonoverlapping(hash.as_ptr(), out_buf, 32);
    0
}
