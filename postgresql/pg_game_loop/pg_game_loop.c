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

PG_MODULE_MAGIC;

void _PG_init(void);
PGDLLEXPORT void orchestrator_main(Datum main_arg);
PG_FUNCTION_INFO_V1(game_loop_run);

static double TickMs = 33.3;
static char* DatabaseName = "postgres";

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
    snprintf(worker.bgw_function_name, BGW_MAXLEN, "orchestrator_main");
    snprintf(worker.bgw_library_name, BGW_MAXLEN, "pg_game_loop");
    worker.bgw_restart_time = 10;
    worker.bgw_flags = BGWORKER_SHMEM_ACCESS | BGWORKER_BACKEND_DATABASE_CONNECTION;
    worker.bgw_start_time = BgWorkerStart_RecoveryFinished;
    worker.bgw_notify_pid = 0;
    RegisterBackgroundWorker(&worker);
    ereport(NOTICE, errmsg("pg_game_loop initialized, tick_ms: %f", TickMs));
}

void orchestrator_main(Datum main_arg) {
    HeapTuple tuple;
    TupleDesc desc;
    char* version;
    pqsignal(SIGHUP, SignalHandlerForConfigReload);
	pqsignal(SIGINT, SIG_IGN);
	pqsignal(SIGTERM, die);
    BackgroundWorkerInitializeConnection(DatabaseName, NULL, 0);
    BackgroundWorkerUnblockSignals();
    ereport(NOTICE, errmsg("game_loop_orchestrator BackgroundWorker started"));
    for(;;) {
        if (proc_exit_inprogress) {
            proc_exit(0);
        }
        StartTransactionCommand();
        PushActiveSnapshot(GetTransactionSnapshot());
        if (SPI_connect() != SPI_OK_CONNECT) {
            ereport(ERROR, errmsg("SPI_connect failed"));
        }
        ereport(NOTICE, errmsg("Fetching Postgres Version"));
        if (SPI_execute("SELECT version()", true, 0) != SPI_OK_SELECT) {
            ereport(ERROR, errmsg("SELECT failed"));
        }
        ereport(NOTICE, errmsg("Version fetched"));
        if (SPI_tuptable != NULL && SPI_processed > 0) {
            ereport(NOTICE, errmsg("Results count: %lu", SPI_processed));
            tuple = SPI_tuptable->vals[0];
            desc = SPI_tuptable->tupdesc;

            version = SPI_getvalue(tuple, desc, 1);
            ereport(NOTICE, errmsg("Postgres Version: %s", version));
        }
        SPI_finish();
        PopActiveSnapshot();
        CommitTransactionCommand();
        pg_usleep(1000 * 1000);
    }
}

Datum game_loop_run(PG_FUNCTION_ARGS) {
    instr_time start, end;
    double deltaMicro;
    double tickMicro;
    pg_uuid_t* game_id;
    char* tick_function;
    char* query;
    Datum values[2];
    Oid argtypes[2] = {UUIDOID, FLOAT8OID};

    ereport(NOTICE, errmsg("C game loop started with tick_ms: %f", TickMs));
    deltaMicro = 0.0;
    tickMicro = TickMs * 1000.0;
    game_id = PG_GETARG_UUID_P(0);
    tick_function = text_to_cstring(PG_GETARG_TEXT_PP(1));
    query = psprintf("SELECT %s($1, $2)", tick_function);
    SPI_connect();
    values[0] = UUIDPGetDatum(game_id);
    while(true) {
        INSTR_TIME_SET_CURRENT(start);
        if (deltaMicro >= tickMicro) {
            deltaMicro = fmod(deltaMicro, tickMicro);
            values[1] = Float8GetDatum(TickMs);
            BeginInternalSubTransaction(NULL);
            if (SPI_execute_with_args(query, 2, argtypes, values, NULL, false, 0) != SPI_OK_SELECT) {
                ereport(ERROR, errmsg("Failed to call %s function", tick_function));
            }
            ReleaseCurrentSubTransaction();
        } else {
            pg_usleep(900L);
        }
        INSTR_TIME_SET_CURRENT(end);
        INSTR_TIME_SUBTRACT(end, start);
        deltaMicro += INSTR_TIME_GET_MICROSEC(end);
    }
    SPI_finish();
    PG_RETURN_VOID();
}