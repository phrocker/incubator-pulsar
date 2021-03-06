/**
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */
package org.apache.bookkeeper.mledger;

import com.google.common.annotations.Beta;

import java.util.Map;
import java.util.UUID;
import java.util.concurrent.CompletableFuture;

import org.apache.bookkeeper.client.api.ReadHandle;

/**
 * Interface for offloading ledgers to longterm storage
 */
@Beta
public interface LedgerOffloader {
    /**
     * Offload the passed in ledger to longterm storage.
     * Metadata passed in is for inspection purposes only and should be stored
     * alongside the ledger data.
     *
     * When the returned future completes, the ledger has been persisted to the
     * loadterm storage, so it is safe to delete the original copy in bookkeeper.
     *
     * The uid is used to identify an attempt to offload. The implementation should
     * use this to deterministically generate a unique name for the offloaded object.
     * This uid will be stored in the managed ledger metadata before attempting the
     * call to offload(). If a subsequent or concurrent call to offload() finds
     * a uid in the metadata, it will attempt to cleanup this attempt with a call
     * to #deleteOffloaded(ReadHandle,UUID). Once the offload attempt completes,
     * the managed ledger will update its metadata again, to record the completion,
     * ensuring that subsequent calls will not attempt to offload the same ledger
     * again.
     *
     * @param ledger the ledger to offload
     * @param uid unique id to identity this offload attempt
     * @param extraMetadata metadata to be stored with the ledger for informational
     *                      purposes
     * @return a future, which when completed, denotes that the offload has been
     *         successful
     */
    CompletableFuture<Void> offload(ReadHandle ledger,
                                    UUID uid,
                                    Map<String, String> extraMetadata);

    /**
     * Create a ReadHandle which can be used to read a ledger back from longterm
     * storage.
     *
     * The passed uid, will be match the uid of a previous successful call to
     * #offload(ReadHandle,UUID,Map).
     *
     * @param ledgerId the ID of the ledger to load from longterm storage
     * @param uid unique ID for previous successful offload attempt
     * @return a future, which when completed, returns a ReadHandle
     */
    CompletableFuture<ReadHandle> readOffloaded(long ledgerId, UUID uid);

    /**
     * Delete a ledger from long term storage.
     *
     * The passed uid, will be match the uid of a previous call to
     * #offload(ReadHandle,UUID,Map), which may or may not have been successful.
     *
     * @param ledgerId the ID of the ledger to delete from longterm storage
     * @param uid unique ID for previous offload attempt
     * @return a future, which when completed, signifies that the ledger has
     *         been deleted
     */
    CompletableFuture<Void> deleteOffloaded(long ledgerId, UUID uid);
}

