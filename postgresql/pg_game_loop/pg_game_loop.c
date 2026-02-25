#include "postgres.h"
#include "fmgr.h"
#include "utils/elog.h"

PG_MODULE_MAGIC;

PG_FUNCTION_INFO_V1(game_loop_run);

Datum game_loop_run(PG_FUNCTION_ARGS) {
    ereport(NOTICE, errmsg("C game loop started"));
    PG_RETURN_VOID();
}