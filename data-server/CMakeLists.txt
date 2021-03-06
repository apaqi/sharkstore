cmake_minimum_required(VERSION 2.6)
project(fbase-ds C CXX)

set(CMAKE_CXX_COMPILER "g++")
set(CMAKE_CXX_STANDARD 11)

set(CXX_FLAGS
    -std=c++11
    -Wall
    -Wextra
    -Wformat
    -Wsign-compare
    -Wno-unused-parameter
    -Wno-unused-variable
    -Woverloaded-virtual
    -Wnon-virtual-dtor
    -Wno-missing-field-initializers
    -D__STDC_FORMAT_MACROS
    -DASIO_STANDALONE
)

string(REPLACE ";" " " CMAKE_CXX_FLAGS "${CXX_FLAGS}")

set(CMAKE_C_FLAGS "-std=c99 -Wall -Wformat -D_FILE_OFFSET_BITS=64 -D_GNU_SOURCE -DOS_LINUX -DIOEVENT_USE_EPOLL")

if(NOT CMAKE_BUILD_TYPE STREQUAL "Debug")
    set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -O2 -DNDEBUG")
    set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -O2 -DNDEBUG" )
    set(CMAKE_C_FLAGS_RELEASE "")
    set(CMAKE_CXX_FLAGS_RELEASE "")
else()
    set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -O0 -g -ggdb -DDEBUG_FLAG")
    set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -O0 -g -ggdb -DDEBUG_FLAG")
endif()
MESSAGE(STATUS BUILD_TYPE=${CMAKE_BUILD_TYPE})

# use gcov
OPTION (ENABLE_COVERAGE "Use gcov" OFF)
MESSAGE(STATUS ENABLE_COVERAGE=${ENABLE_COVERAGE})
if(ENABLE_COVERAGE)
    set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -fprofile-arcs -ftest-coverage")
    set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -fprofile-arcs -ftest-coverage")
endif()

# profiler
OPTION (ENABLE_GPERF "Use gperf" OFF)
MESSAGE(STATUS ENABLE_GPERF=${ENABLE_GPERF})
if(ENABLE_GPERF)
    set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -DUSE_GPERF")
    set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -DUSE_GPERF" )
endif()

# gcc address sanitize
OPTION (ENABLE_SANITIZE "Use gcc address sanitize" OFF)
MESSAGE(STATUS ENABLE_SANITIZE=${ENABLE_SANITIZE})
if(ENABLE_SANITIZE )
    set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -fsanitize=address")
    set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -fsanitize=address" )
endif()



include_directories(${PROJECT_SOURCE_DIR}/src)
include_directories(${PROJECT_SOURCE_DIR}/src/raft/include)

find_package(Protobuf REQUIRED)
include_directories(${PROTOBUF_INCLUDE_DIRS})


set(SOURCES
    src/server/main.cpp
    src/server/callback.cpp
    src/server/server.cpp
    src/server/worker.cpp
    src/server/manager.cpp
    src/server/node_address.cpp
    src/server/raft_logger.cpp
    src/server/run_status.cpp
    src/server/range_server.cpp
    src/range/range.cpp
    src/range/lock.cpp
    src/range/raw_get.cpp
    src/range/raw_put.cpp
    src/range/raw_delete.cpp
    src/range/select.cpp
    src/range/insert.cpp
    src/range/delete.cpp
    src/range/split.cpp
    src/range/peer.cpp
    src/range/snapshot.cpp
    src/range/kv_funcs.cpp
    src/storage/aggregate_calc.cpp
    src/storage/field_value.cpp
    src/storage/iterator.cpp
    src/storage/meta_store.cpp
    src/storage/metric.cpp
    src/storage/row_decoder.cpp
    src/storage/row_fetcher.cpp
    src/storage/store.cpp
    src/master/client.cpp
    src/master/connection.cpp
    src/master/rpc_types.cpp
    src/master/worker.cpp
    src/monitor/isystemstatus.cpp
    src/monitor/linuxstatus.cpp
    src/monitor/datacacl.cpp
    src/monitor/encodedata.cpp
    src/monitor/rangedata.cpp
)

add_subdirectory(src/base)
add_subdirectory(src/lk_queue)
add_subdirectory(src/common)
add_subdirectory(src/frame)
add_subdirectory(src/proto)
add_subdirectory(src/raft)
add_subdirectory(src/tool)

OPTION (BUILD_NET "build fbase net" OFF)
MESSAGE(STATUS BUILD_NET=${BUILD_NET})
if (BUILD_NET)
    add_subdirectory(src/net)
endif()

set (depend_LIBRARYS
    fbase-base
    fbase-proto 
    fbase-raft
    fbase-common
    fbase-frame
    fbase-lkqueue
    ${PROTOBUF_LIBRARY}
    grpc
    grpc++
    fastcommon
    rocksdb
    pthread
    dl
    z
)
if(ENABLE_GPERF)
    list(APPEND depend_LIBRARYS profiler)
endif()

foreach(f IN LISTS SOURCES) 
    # remove "src/" 
    string(SUBSTRING ${f} 4 -1 fname) 
    set_source_files_properties(${f} PROPERTIES 
        COMPILE_DEFINITIONS "__FNAME__=\"${fname}\"") 
endforeach() 

add_executable(data-server ${SOURCES})
target_link_libraries(data-server ${depend_LIBRARYS})

OPTION (BUILD_TEST "build fbase tests" OFF)
MESSAGE(STATUS BUILD_TEST=${BUILD_TEST})
if (BUILD_TEST)
    enable_testing()
    find_package(GTest REQUIRED)
    include_directories(${GTEST_INCLUDE_DIRS})
    add_library(fbase-ds-shared SHARED ${SOURCES})
    add_subdirectory(test)
endif()
