#include <stdio.h>
#include <stdbool.h>
#include <stdint.h>
#include <time.h>
#include "../../../../../../c-deps/libplsql/include/utils/numeric.h"

typedef uint32_t uint32;
typedef uint32 Oid;
typedef int64_t int64;

#define DATE 0
#define TIME 1
#define TIMESTAMP 2
#define TIMESTAMPTZ 3

typedef struct {
    unsigned long engine;
    unsigned long pointer;
    unsigned long cursorEngine;
    // unsigned long tempEngine;
} Pointer_vec;

// core pointer UDRRumParam from drdb
typedef struct {
    //the return rc
    int rc;
    // the error msg of ret
    char *ErrMsg;
    //the engine to get params
    // DBEngine *tempEngine;
    //save znbase funcDesc 's addr
    unsigned long Pointer_func_desc;
    // save znbase UDRRunParam's address
    unsigned long pointer;
    // cursor temp engine (rocksDB)
    unsigned long cursorEngine;
    // save a magic code generated by random in caseof UDRRumParam is corrupted by golang
    int magicCode;
} Handler_t;

// Param List info struct
typedef struct _ParamListInfo {
    void *parserSetupArg;

} _ParamListInfo;
typedef struct _ParamListInfo *ParamListInfo;

// cursor Desc
typedef struct CursorDesc {
    char *name;     /* varlena header (do not touch directly!) */
    uint32 id;      /* cursor id */
    char *queryStr; /* cursor's source query string */
    int64 position; /* cursor position */
    void *columnDescs; /* cursor's columnDescs */
    void *plan;        /* # cursor plan store with type void* */
} CursorDesc, *CursorDescPtr;

typedef struct cursorArg_name_type {
    char *name;
    char *type_name;
    Oid typoid;
} cursorArg_name_type;

// basic typedef
typedef enum FetchDirection {
    /* for these, howMany is how many rows to fetch; FETCH_ALL means ALL */
    FETCH_FORWARD,
    FETCH_BACKWARD,
    /* for these, howMany indicates a position; only one row is fetched */
    FETCH_ABSOLUTE,
    FETCH_RELATIVE
} FetchDirection;

typedef unsigned int uint;
typedef uint BEPIPlanIndex;

typedef int *BEPI_BOOL_PTR;
typedef int *BEPI_INT_PTR;
typedef float *BEPI_FLOAT_PTR;
typedef char *BEPI_STRING_PTR;
typedef char *BEPI_BYTES_PTR;
typedef struct BEPI_ARRAY_INT {
    int *vals;
    int num;
} BEPI_ARRAY_INT, *BEPI_ARRAY_INT_PTR;
typedef struct BEPI_ARRAY_FLOAT {
    float *vals;
    int num;
} BEPI_ARRAY_FLOAT, *BEPI_ARRAY_FLOAT_PTR;
typedef struct BEPI_ARRAY_STRING {
    char **vals;
    int num;
} BEPI_ARRAY_STRING, *BEPI_ARRAY_STRING_PTR;
typedef struct BEPI_BIT {
    unsigned int vals: 1;
} BEPI_BIT, *BEPI_BIT_PTR;
typedef struct BEPI_VARBIT {
    int num;
    unsigned int *vals;
} BEPI_VARBIT, *BEPI_VARBIT_PTR;

typedef struct {
    struct tm tm;
    int time_type;
    int microseconds;
} BEPI_TIMESTAMP;

//extern int BEPI_execute_plan_with_paramlist(Handler_t handler, BEPIPlanIndex planIndex,
//                                     ParamListInfo params,
//                                     bool read_only, long tcount, bool *is_query_or_hasReturning);

extern BEPIPlanIndex BEPI_prepare(Handler_t handler, const char *src);

extern BEPIPlanIndex BEPI_prepare_params(Handler_t handler, const char *src, int cursorOptions);

extern int BEPI_getargcount(Handler_t handler);

extern uint32 BEPI_getargtypeid(Handler_t handler, int argIndex);

extern int
BEPI_cursor_declare(Handler_t handler, const char *cursorName, char *query, cursorArg_name_type *curArg, int arg_num);

extern int BEPI_cursor_open_with_paramlist(Handler_t handler, const char *name,
                                           ParamListInfo params, char *arg_str, bool read_only);

extern CursorDesc BEPI_cursor_find(Handler_t handler, const char *name, char **errmsg);

extern void BEPI_cursor_move(Handler_t handler, char *name, FetchDirection direction, long count);

extern bool BEPI_cursor_fetch(Handler_t handler, char *name, FetchDirection direction, long count);

extern void BEPI_cursor_close(Handler_t handler, char *name, char *errmsg);

extern int BEPI_execute(Handler_t handler, const char *src);

extern int BEPI_exec(Handler_t handler, const char *src);

extern int BEPI_execute_plan(Handler_t handler, BEPIPlanIndex planIndex);

extern int BEPI_execp(Handler_t handler, BEPIPlanIndex planIndex);

char *BEPI_castValue_toType(Handler_t handler, char *valStr, char **resStr, int srcOid, int castOid);

extern void *BEPI_palloc(int size);

extern void BEPI_pfree(void *pointer);

typedef long date;

extern date *PGTYPESdate_new(void);

extern void PGTYPESdate_free(date *);

extern BEPI_TIMESTAMP *BEPI_timestamp_new();

extern BEPI_TIMESTAMP *BEPI_str2ts(char *str);

extern char *BEPI_ts2str(const char *, const BEPI_TIMESTAMP *);

extern numeric *TYPESnumeric_new(void);

extern decimal *TYPESdecimal_new(void);

extern void TYPESnumeric_free(numeric *);

extern void TYPESdecimal_free(decimal *);

extern numeric *TYPESnumeric_from_asc(char *, char **);

extern char *TYPESnumeric_to_asc(numeric *, int);

extern int TYPESnumeric_add(numeric *, numeric *, numeric *);

extern int TYPESnumeric_sub(numeric *, numeric *, numeric *);

extern int TYPESnumeric_mul(numeric *, numeric *, numeric *);

extern int TYPESnumeric_div(numeric *, numeric *, numeric *);

extern int TYPESnumeric_cmp(numeric *, numeric *);

extern int TYPESnumeric_from_int(signed int, numeric *);

extern int TYPESnumeric_from_long(signed long int, numeric *);

extern int TYPESnumeric_copy(numeric *, numeric *);

extern int TYPESnumeric_from_double(double, numeric *);

extern int TYPESnumeric_to_double(numeric *, double *);

extern int TYPESnumeric_to_int(numeric *, int *);

extern int TYPESnumeric_to_long(numeric *, long *);

extern int TYPESnumeric_to_decimal(numeric *, decimal *);

extern int TYPESnumeric_from_decimal(decimal *, numeric *);