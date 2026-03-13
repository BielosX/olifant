#include <math.h>
#include <string.h>
#include "postgres.h"
#include "fmgr.h"
#include "utils/elog.h"
#include "utils/guc.h"
#include "executor/spi.h"
#include "miscadmin.h"
#include "utils/uuid.h"
#include "utils/builtins.h"
#include "portability/instr_time.h"
#include "postmaster/bgworker.h"
#include "postmaster/interrupt.h"
#include "storage/latch.h"
#include "storage/ipc.h"
#include "storage/proc.h"
#include "tcop/tcopprot.h"
#include "utils/wait_classes.h"
#include "commands/extension.h"
#include "port.h"
#include "utils/timestamp.h"
#include "utils/memutils.h"

PG_MODULE_MAGIC;

void _PG_init(void);
PGDLLEXPORT void orchestratorMain(Datum main_arg);
PGDLLEXPORT void workerMain(Datum main_arg);

static double TickMs = 33.3;
static char* DatabaseName = "postgres";
static char* ExtensionName = "pg_game_loop";

void _PG_init() {
    struct BackgroundWorker worker;
    DefineCustomRealVariable(
        "game_loop.tick_ms",
        gettext_noop("Game loop tick period."),
        NULL,
        &TickMs,
        50.0,
        1.0,
        1000.0,
        PGC_SUSET,
        0,
        NULL, NULL, NULL
    );
    DefineCustomStringVariable(
        "game_loop.db_name",
        gettext_noop("Game loop database name."),
        NULL,
        &DatabaseName,
        "postgres",
        PGC_SUSET,
        0,
        NULL, NULL, NULL
    );
    snprintf(worker.bgw_name, BGW_MAXLEN, "game_loop_orchestrator");
    snprintf(worker.bgw_type, BGW_MAXLEN, "game_loop");
    snprintf(worker.bgw_function_name, BGW_MAXLEN, "orchestratorMain");
    snprintf(worker.bgw_library_name, BGW_MAXLEN, "pg_game_loop");
    worker.bgw_restart_time = 10;
    worker.bgw_flags = BGWORKER_SHMEM_ACCESS | BGWORKER_BACKEND_DATABASE_CONNECTION;
    worker.bgw_start_time = BgWorkerStart_RecoveryFinished;
    worker.bgw_notify_pid = 0;
    RegisterBackgroundWorker(&worker);
    ereport(NOTICE, errmsg("pg_game_loop initialized, tick_ms: %f", TickMs));
}

static void initWorker() {
    pqsignal(SIGHUP, SignalHandlerForConfigReload);
	pqsignal(SIGINT, SIG_IGN);
	pqsignal(SIGTERM, die);
    BackgroundWorkerInitializeConnection(DatabaseName, NULL, 0);
    BackgroundWorkerUnblockSignals();
}

static char* getUUIDStr(pg_uuid_t* uuid) {
    return DatumGetCString(DirectFunctionCall1(uuid_out, UUIDPGetDatum(uuid)));
}

#define UPDATE_FUNC_NAME_MAX_LEN 64

typedef struct WorkerArgs {
    pg_uuid_t gameId;
    char updateFuncName[UPDATE_FUNC_NAME_MAX_LEN];
} WorkerArgs;

static void runGameLoopWorker(pg_uuid_t* gameId, char* updateFuncName) {
    struct BackgroundWorker worker;
    struct WorkerArgs args;
    char *uuid_str = getUUIDStr(gameId);
    snprintf(worker.bgw_name, BGW_MAXLEN, "game_loop_worker_%s", uuid_str);
    snprintf(worker.bgw_type, BGW_MAXLEN, "game_loop");
    snprintf(worker.bgw_function_name, BGW_MAXLEN, "workerMain");
    snprintf(worker.bgw_library_name, BGW_MAXLEN, "pg_game_loop");
    worker.bgw_restart_time = 10;
    worker.bgw_flags = BGWORKER_SHMEM_ACCESS | BGWORKER_BACKEND_DATABASE_CONNECTION;
    worker.bgw_start_time = BgWorkerStart_RecoveryFinished;
    worker.bgw_notify_pid = MyProcPid;
    memcpy(&args.gameId, gameId, sizeof(pg_uuid_t));
    strlcpy(args.updateFuncName, updateFuncName, UPDATE_FUNC_NAME_MAX_LEN);
    memcpy(worker.bgw_extra, &args, sizeof(WorkerArgs));
    pfree(uuid_str);
    RegisterDynamicBackgroundWorker(&worker, NULL);
}

static bool isExtensionLoaded(void) {
    Oid extensionOid;
    StartTransactionCommand();
    extensionOid = get_extension_oid(ExtensionName, true);
    CommitTransactionCommand();
    if (extensionOid == InvalidOid) {
        return false;
    }
    if (creating_extension && CurrentExtensionObject == extensionOid) {
        return false;
    }
    if (IsBinaryUpgrade) {
        return false;
    }
    return true;
}

static void waitForLatch(int timeoutMs) {
	int rc = 0;
	int waitFlags = WL_LATCH_SET | WL_POSTMASTER_DEATH | WL_TIMEOUT;
	rc = WaitLatch(MyLatch, waitFlags, timeoutMs, PG_WAIT_EXTENSION);
	ResetLatch(MyLatch);
	CHECK_FOR_INTERRUPTS();
	if (rc & WL_POSTMASTER_DEATH) {
		proc_exit(1);
	}
}

static bool isGameFinished(pg_uuid_t* gameId) {
    Datum values[1];
    Oid argtypes[1] = {UUIDOID};
    int result = 0;
    bool isNull;
    Datum countResult;
    values[0] = UUIDPGetDatum(gameId);
    result = SPI_execute_with_args(
        "SELECT count(*) FROM game_loop.games WHERE id=$1 AND game_state='FINISHED'",
        1,
        argtypes,
        values,
        NULL,
        true,
        1
    );
    if (result != SPI_OK_SELECT) {
        ereport(ERROR, errmsg("Failed to get the game state"));
    }
    if (SPI_tuptable != NULL && SPI_processed > 0) {
        countResult = SPI_getbinval(SPI_tuptable->vals[0], SPI_tuptable->tupdesc, 1, &isNull);
        SPI_freetuptable(SPI_tuptable);
        if (!isNull) {
            return DatumGetInt64(countResult) > 0;
        }
    }
    return true;
}

static void notifyClient(pg_uuid_t* gameId) {
    StringInfoData buf;
    char *uuid_str;
    uuid_str = getUUIDStr(gameId);
    ereport(NOTICE, errmsg("Sending notification to %s", uuid_str));
    initStringInfo(&buf);
    appendStringInfo(&buf, "NOTIFY \"%s\"", uuid_str);
    if(SPI_execute(buf.data, false, 0) != SPI_OK_UTILITY) {
        ereport(ERROR, errmsg("NOTIFY failed"));
    }
    resetStringInfo(&buf);
    pfree(uuid_str);
}

static void callUpdateFunction(double delta, pg_uuid_t* gameId, char* functionName) {
    StringInfoData buf;
    Datum values[2];
    Oid argtypes[2] = {UUIDOID, FLOAT8OID};
    initStringInfo(&buf);
    appendStringInfo(&buf, "SELECT %s($1,$2)", functionName);
    values[0] = UUIDPGetDatum(gameId);
    values[1] = Float8GetDatum(delta);
    if (SPI_execute_with_args(buf.data, 2, argtypes, values, NULL, false, 0) != SPI_OK_SELECT) {
        ereport(ERROR, errmsg("Failed to call %s function", functionName));
    }
    resetStringInfo(&buf);
}

void workerMain(Datum mainArg) {
    WorkerArgs *args;
    pg_uuid_t *gameId;
    char *uuid_str;
    int64 updates;
    int i;
    instr_time start, end;
    int64 deltaMicro;
    int64 tickMicro;
    args = (WorkerArgs*)MyBgworkerEntry->bgw_extra;
    initWorker();
    gameId = &args->gameId;
    uuid_str = getUUIDStr(&args->gameId);
    ereport(NOTICE, errmsg("%s BackgroundWorker started for game %s", MyBgworkerEntry->bgw_name, uuid_str));
    deltaMicro = 0;
    tickMicro = TickMs * 1000;
    for (;;) {
        INSTR_TIME_SET_CURRENT(start);
        StartTransactionCommand();
        PushActiveSnapshot(GetTransactionSnapshot());
        if (SPI_connect() != SPI_OK_CONNECT) {
            ereport(ERROR, errmsg("SPI_connect failed"));
        }
        if (isGameFinished(gameId)) {
            ereport(NOTICE, errmsg("Game %s finished", uuid_str));
            proc_exit(0);
        }
        if (deltaMicro > tickMicro) {
            updates = deltaMicro / tickMicro;
            deltaMicro = deltaMicro % tickMicro;
            for (i = 0; i < updates; i++) {
                callUpdateFunction(TickMs, gameId, args->updateFuncName);
            }
            notifyClient(gameId);
        } else {
            pg_usleep(500L);
        }
        SPI_finish();
        PopActiveSnapshot();
        CommitTransactionCommand();
        INSTR_TIME_SET_CURRENT(end);
        INSTR_TIME_SUBTRACT(end, start);
        deltaMicro += INSTR_TIME_GET_MICROSEC(end);
    }
}

static void setRunningState(pg_uuid_t* gameId) {
    Datum values[2];
    Oid argtypes[2] = {TIMESTAMPTZOID, UUIDOID};
    values[0] = TimestampTzGetDatum(GetCurrentTimestamp());
    values[1] = UUIDPGetDatum(gameId);
    SPI_execute_with_args(
        "UPDATE game_loop.games SET game_state='RUNNING', last_update=$1 WHERE id=$2",
        2,
        argtypes,
        values,
        NULL,
        false,
        0
    );
    SPI_freetuptable(SPI_tuptable);
}

void orchestratorMain(Datum main_arg) {
    pg_uuid_t *gameId;
    char* updateFunction;
    HeapTuple tuple; 
    TupleDesc tupdesc;
    int i;
    bool isNull;
    char* query = "SELECT id, update_function FROM game_loop.games WHERE game_state='INITIATED'";
    initWorker();
    while (!isExtensionLoaded()) {
        ereport(NOTICE, errmsg("Extension %s not fully loaded, waiting...", ExtensionName));
        waitForLatch(1000);
    }
    ereport(NOTICE, errmsg("Extension %s fully loaded", ExtensionName));
    ereport(NOTICE, errmsg("game_loop_orchestrator BackgroundWorker started"));
    for(;;) {
        StartTransactionCommand();
        PushActiveSnapshot(GetTransactionSnapshot());
        if (SPI_connect() != SPI_OK_CONNECT) {
            ereport(ERROR, errmsg("SPI_connect failed"));
        }
        ereport(NOTICE, errmsg("Fetching Initialized games"));
        if (SPI_execute(query, true, 0) != SPI_OK_SELECT) {
            ereport(ERROR, errmsg("Unable to fetch initialized games"));
        }
        if (SPI_tuptable != NULL && SPI_processed > 0) {
            ereport(NOTICE, errmsg("%ld games found, starting", SPI_processed));
            tupdesc = SPI_tuptable->tupdesc;
            for(i = 0; i < SPI_processed; i++) {
                tuple = SPI_tuptable->vals[i];
                gameId = DatumGetUUIDP(SPI_getbinval(tuple, tupdesc, 1, &isNull));
                updateFunction = SPI_getvalue(tuple, tupdesc, 2);
                ereport(NOTICE, errmsg("Using updateFunction %s", updateFunction));
                ereport(NOTICE, errmsg("Starting a game with ID %s", getUUIDStr(gameId)));
                runGameLoopWorker(gameId, updateFunction);
                SPI_freetuptable(SPI_tuptable);
                setRunningState(gameId);
            }
        } else {
            ereport(NOTICE, errmsg("No games to start"));
        }
        SPI_finish();
        PopActiveSnapshot();
        CommitTransactionCommand();
        waitForLatch(1000);
    }
}