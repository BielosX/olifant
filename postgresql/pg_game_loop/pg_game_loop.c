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

PG_MODULE_MAGIC;

void _PG_init(void);
PG_FUNCTION_INFO_V1(game_loop_run);

static double TickMs = 33.3;

void _PG_init() {
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
    ereport(NOTICE, errmsg("pg_game_loop initialized, tick_ms: %f", TickMs));
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