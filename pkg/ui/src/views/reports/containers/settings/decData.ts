let mapDec = new Map<string, string>();
mapDec.set("changefeed.experimental_poll_interval","changefeed原型实现的轮询间隔（警告：可能会损害集群的稳定性或正确性；请勿在没有监督的情况下进行编辑）");
mapDec.set("changefeed.push.enabled","如果已设置，则更改将会保存。 这需要kv.rangefeed.enabled设置");
mapDec.set( "cloudsink.http.custom_ca","自定义root CA（附加到系统的默认CA），用于在与HTTPS存储进行交互时验证证书");
mapDec.set("cloudsink.timeout","加载/导出储存储操作的超时");
mapDec.set("cluster.organization","机构名称");
mapDec.set("cluster.preserve_downgrade_option","从指定版本禁用（自动或手动）集群版本升级，直到重置");
mapDec.set( "compactor.enabled","如果为false，则系统将不太积极地回收已删除数据所占用的空间");
mapDec.set("compactor.max_record_age","丢弃在此期间未处理的建议（警告：可能会损害集群的稳定性或正确性；请勿在没有监督的情况下进行编辑）");
mapDec.set("compactor.min_interval","压缩之前要等待的最短时间间隔（警告：可能会损害集群的稳定性或正确性；请勿在没有监督的情况下进行编辑）");
mapDec.set("compactor.threshold_available_fraction","考虑至少针对给定百分比的可用逻辑空间的建议（禁用为零）（警告：可能会损害集群的稳定性或正确性；请勿在没有监督的情况下进行编辑）");
mapDec.set("compactor.threshold_bytes","在考虑汇总建议之前需要的最低预期逻辑空间回收（警告：可能会损害集群的稳定性或正确性；请勿在未经监督的情况下进行编辑）");
mapDec.set("compactor.threshold_used_fraction","考虑至少使用逻辑空间的给定百分比的建议（禁用为零）（警告：可能会损害集群的稳定性或正确性；请勿在没有监督的情况下进行编辑）");
mapDec.set("debug.panic_on_failed_assertions","当断言失败而不是报告时会产生panic");
mapDec.set( "diagnostics.forced_stat_reset.interval","即使没有报告，也应丢弃待处理的诊断统计信息的时间间隔");
mapDec.set(  "diagnostics.reporting.enabled","启用向znbase实验室报告诊断指标");
mapDec.set("diagnostics.reporting.interval","报告诊断数据的时间间隔（应短于diagnostics.forced_stat_reset.interval）");
mapDec.set("diagnostics.reporting.send_crash_reports","发送崩溃和紧急报告");
mapDec.set( "external.graphite.endpoint","如果为非空，则将服务器指标推送到指定主机上的Graphite或Carbon服务器：端口");
mapDec.set( "external.graphite.interval","将指标推送到Graphite的时间间隔");
mapDec.set( "jobs.registry.leniency","推迟尝试重新安排工作的时间");
mapDec.set("jobs.retention_time","保留之前完成的工作记录的时间");
mapDec.set("kv.allocator.lease_rebalancing_aggressiveness","设置大于1.0可以更积极地使租赁重新平衡以适应负载，或者设置为0到1.0之间可以使租赁重新平衡更加保守");
mapDec.set( "kv.allocator.load_based_lease_rebalancing.enabled","设置为基于负载和延迟启用范围租约的重新平衡");
mapDec.set("kv.allocator.load_based_rebalancing","是否根据store之间的QPS分布进行重新平衡[off = 0, leases = 1, leases and replicas = 2]");
mapDec.set( "kv.allocator.qps_rebalance_threshold","与store的QPS（例如每秒查询数）的平均值相差的最小分数可以被认为是过满或不足");
mapDec.set("kv.allocator.range_rebalance_threshold","距离store范围数均值的最小分数");
mapDec.set( "kv.bulk_io_write.addsstable_max_rate","单个store每秒的最大AddSSTable请求数");
mapDec.set("kv.bulk_io_write.concurrent_addsstable_requests","store在排队之前将同时处理的AddSSTable请求数");
mapDec.set( "kv.bulk_io_write.concurrent_export_requests","store在排队之前将同时处理的导出请求数");
mapDec.set("kv.bulk_io_write.concurrent_import_requests","store在排队之前将同时处理的导入请求数");
mapDec.set("kv.bulk_io_write.max_rate","代表批量io ops用于写入磁盘的速率限制（字节/秒）");
mapDec.set("kv.bulk_sst.sync_size","非Rocket SST写入必须达到fsync的阈值（禁用0)");
mapDec.set("kv.closed_timestamp.close_fraction","关闭时间戳记的目标持续时间的分数，指定提前关闭时间戳记的频率");
mapDec.set("kv.closed_timestamp.follower_reads_enabled","允许（所有）副本基于封闭的时间戳信息提供一致的历史读取");
mapDec.set( "kv.closed_timestamp.target_duration","如果不为零，请尝试提供大约在此持续时间后跟集群时间的时间戳的封闭时间戳通知");
mapDec.set("kv.follower_read.target_multiple","如果大于1，则如果请求早于kv.closed_timestamp.target_duration *（1 + kv.closed_timestamp.close_fraction * this）小于时钟不确定时间间隔，则鼓励distsender对最近的副本执行读取。 此值也用于创建follower_timestamp（）。 （警告：可能会损害集群的稳定性或正确性；请勿在未经监督的情况下进行编辑）");
mapDec.set( "kv.load.batch_size","AddSSTable请求中有效负载的最大大小（警告：可能会损害集群的稳定性或正确性；请勿在没有监督的情况下进行编辑）");
mapDec.set( "kv.raft.command.max_size","raft命令的最大值");
mapDec.set( "kv.raft_log.disable_synchronization_unsafe","设置为true可禁用将Raft日志写入持久性存储时的同步。 设置为true可能会导致服务器崩溃时数据丢失或数据损坏的风险。 该设置仅用于内部测试，不应在生产中使用。");
mapDec.set("kv.range.backpressure_range_size_multiplier","允许范围扩大到的range_max_bytes的倍数，在阻止对该范围的写入之前将其拆分，否则为0禁用");
mapDec.set("kv.range_descriptor_cache.size","范围描述符和租约持有人缓存中的最大条目数");
mapDec.set("kv.range_merge.queue_enabled","是否启用自动合并队列");
mapDec.set("kv.range_merge.queue_interval","合并队列在处理副本之间等待的时间（警告：可能会损害集群的稳定性或正确性；请勿在没有监督的情况下进行编辑）");
mapDec.set( "kv.range_split.by_load_enabled","允许根据负载集中的位置自动划分范围");
mapDec.set( "kv.range_split.load_qps_threshold","QPS，范围变为基于负载的拆分的候选者");
mapDec.set("kv.rangefeed.concurrent_catchup_iterators","store在排队之前可以同时允许的rangefeeds追赶迭代器的数量");
mapDec.set( "kv.rangefeed.enabled","如果设置，则启用范围输入注册");
mapDec.set("kv.snapshot_rebalance.max_rate","用于重新平衡和向上复制快照的速率限制（字节/秒）");
mapDec.set( "kv.snapshot_recovery.max_rate","恢复快照使用的速率限制（字节/秒）");
mapDec.set( "kv.transaction.max_intents_bytes","用于跟踪事务中的写意图的最大字节数");
mapDec.set(  "kv.transaction.max_refresh_attempts","单个事务批处理可以触发刷新跨度尝试的最大次数");
mapDec.set("kv.transaction.max_refresh_spans_bytes","用于跟踪可序列化事务中的刷新范围的最大字节数");
mapDec.set("kv.transaction.parallel_commits_enabled","如果启用，事务提交将与事务写入并行化");
mapDec.set("kv.transaction.write_pipelining_enabled","如果启用，事务写入将通过Raft共识进行流水线处理");
mapDec.set("kv.transaction.write_pipelining_max_batch_size","如果不为零，则定义将通过Raft共识进行流水处理的最大大小批次");
mapDec.set( "kv.transaction.write_pipelining_max_outstanding_size","禁用流水线之前用于跟踪进行中的流水线写入的最大字节数");
mapDec.set( "rocksdb.ingest_backpressure.delay_l0_file","L0中每个文件的SST提取添加延迟超过配置的限制");
mapDec.set("rocksdb.ingest_backpressure.l0_file_count_threshold","背压SST摄取后的L0文件数");
mapDec.set( "rocksdb.ingest_backpressure.max_delay","一次SST摄入可最大程度地反压");
mapDec.set("rocksdb.ingest_backpressure.pending_compaction_threshold","等待压实估计，高于该压力可反压SST摄入");
mapDec.set(  "rocksdb.min_wal_sync_interval","RocksDB WAL同步之间的最短持续时间");
mapDec.set( "schemachanger.backfiller.buffer_size","回填期间要在内存中缓冲的数量");
mapDec.set( "schemachanger.backfiller.max_sst_size","回填期间提取文件的目标大小");
mapDec.set("schemachanger.bulk_index_backfill.batch_size","批量索引回填期间一次要处理的行数");
mapDec.set("schemachanger.bulk_index_backfill.enabled","通过addstable批量回填索引");
mapDec.set( "schemachanger.lease.duration","模式更改租约的期限");
mapDec.set( "schemachanger.lease.renew_fraction","剩余的schemachanger.lease_duration分数可触发续约");
mapDec.set("server.clock.forward_jump_check_enabled","如果启用，则前向时钟跳变> max_offset / 2会引起恐慌");
mapDec.set( "server.clock.persist_upper_bound_interval","保持时钟的墙壁时间上限之间的时间间隔。 时钟不会产生比持续时间的时间戳大的挂钟时间，并且如果看到的挂钟时间大于此值，将会慌乱。 当znbase启动时，它会等待时间延长，直到此时间戳持续存在。 这样可以保证服务器重启之间的单调时间。 不设置此值或将其设置为0将禁用此功能。");
mapDec.set("server.consistency_check.interval","范围一致性检查之间的时间； 设置为0以禁用一致性检查");
mapDec.set("server.declined_reservation_timeout","保留被拒绝后，考虑将store限制进行向上复制的时间");
mapDec.set("server.eventlog.ttl","如果非零，则每10m0s删除比该持续时间长的事件日志条目。 不应降低到24小时以下。");
mapDec.set( "server.failed_reservation_timeout","预订呼叫失败后，考虑将存储限制用于向上复制的时间");
mapDec.set("server.goroutine_dump.num_goroutines_threshold","阈值，如果goroutine数量增加，则可以触发goroutine转储");
mapDec.set("server.goroutine_dump.total_dump_size_limit","	要保留的goroutine转储的总大小。 将按照创建时间的顺序对转储进行GC。 即使最新转储的大小超过限制，也始终保留");
mapDec.set( "server.heap_profile.max_profiles","保留的最大个人资料数量。 得分较低的个人资料会进行GC，但始终会保留最新的个人资料。");
mapDec.set("server.heap_profile.system_memory_threshold_fraction","系统内存的一小部分，如果Rss超出该部分，则会触发堆配置文件");
mapDec.set( "server.host_based_authentication.configuration","在连接身份验证期间使用的基于主机的身份验证配置");
mapDec.set("server.rangelog.ttl","如果不为零，则早于此持续时间的范围日志条目每10m0s删除一次。 不应降低到24小时以下。");
mapDec.set("server.remote_debugging.mode","设置为启用远程调试，仅限本地主机或禁用（any，local，off）");
mapDec.set("server.shutdown.drain_wait","在继续执行其余关闭过程之前，服务器在未就绪状态下等待的时间");
mapDec.set("server.shutdown.query_wait","服务器将至少等待此时间，以完成活动查询");
mapDec.set( "server.time_until_store_dead","如果没有关于store的新gossip信息，则该时间将视为已失效");
mapDec.set( "server.web_session_timeout","新创建的Web会话有效的持续时间");
mapDec.set( "sql.defaults.default_int_size","INT类型的大小（以字节为单位）");
mapDec.set("sql.defaults.distsql","默认的分布式SQL执行模式 [off = 0); auto = 1); on = 2]");
mapDec.set( "sql.defaults.experimental_vectorize","默认“ experimental_vectorize”模式 [off = 0); on = 1); always = 2]");
mapDec.set("sql.defaults.optimizer","默认的基于成本的优化器模式 [off = 0); on = 1); local = 2]");
mapDec.set("sql.defaults.reorder_joins_limit","要重新排序的默认联接数");
mapDec.set("sql.defaults.results_buffer.size","缓冲区的默认大小，该缓冲区在将一条语句或一批语句的结果发送到客户端之前会对其进行累加。 可以在单个连接上使用'results_buffer_size'参数覆盖此参数。 请注意，自动重试通常仅在没有结果交付给客户端时才会发生，因此减小此大小会增加客户端收到的可重试错误的数量。 另一方面，增加缓冲区大小可能会增加延迟，直到客户端收到第一个结果行。 更新设置仅影响新连接。 设置为0将禁用任何缓冲。");
mapDec.set("sql.defaults.serial_normalization","表定义中SERIAL的默认处理 [rowid = 0); virtual_sequence = 1); sql_sequence = 2]");
mapDec.set("sql.distsql.distribute_index_joins","如果设置，对于索引连接，我们在具有流的每个节点上实例化连接读取器； 如果未设置，则使用单个联接读取器");
mapDec.set("sql.distsql.flow_stream_timeout","输入流在错误建立之前等待流建立的时间");
mapDec.set("sql.distsql.interleaved_joins.enabled","如果设置，我们计划在可能的情况下交错表联接而不是合并联接");
mapDec.set(  "sql.distsql.max_running_flows","节点上可以运行的最大并发流数");
mapDec.set("sql.distsql.merge_joins.enabled","如果已设置，我们将在可能的情况下计划合并联接");
mapDec.set( "sql.distsql.temp_storage.joins","设置为true以启用将磁盘用于分布式sql连接");
mapDec.set( "sql.distsql.temp_storage.sorts","设置为true以启用将磁盘用于分布式sql排序");
mapDec.set("sql.distsql.temp_storage.workmem","退回临时存储之前，处理器可以使用的最大内存量（以字节为单位）");
mapDec.set(  "sql.metrics.statement_details.dump_to_logs","定期清除收集的语句统计信息到节点日志时");
mapDec.set("sql.metrics.statement_details.enabled","收集每个陈述的查询统计信息");
mapDec.set("sql.metrics.statement_details.plan_collection.enabled","定期为每个指纹保存一个逻辑计划");
mapDec.set("sql.metrics.statement_details.plan_collection.period","收集新的逻辑计划之前的时间");
mapDec.set("sql.metrics.statement_details.threshold","导致收集统计信息的最短执行时间");
mapDec.set("sql.parallel_scans.enabled","当可以得出最大结果大小时，并行扫描不同的范围");
mapDec.set( "sql.query_cache.enabled","启用查询缓存");
mapDec.set("sql.stats.automatic_collection.enabled","自动统计收集模式");
mapDec.set( "sql.stats.automatic_collection.fraction_stale_rows","每个表的过时行的目标部分，这将触发统计信息刷新");
mapDec.set( "sql.stats.automatic_collection.max_fraction_idle","自动统计信息采样器处理器处于空闲状态的最大时间比例");
mapDec.set("sql.stats.automatic_collection.min_stale_rows","目标每个表的过时行的最小数量，这将触发统计信息刷新");
mapDec.set("sql.stats.post_events.enabled","如果设置，将为每个CREATE STATISTICS作业显示一个事件");
mapDec.set("sql.tablecache.lease.refresh_limit","定期刷新其租约的最大表数");
mapDec.set("sql.trace.log_statement_execute","设置为true以启用对执行语句的记录");
mapDec.set("sql.trace.session_eventlog.enabled","设置为true以启用会话跟踪");
mapDec.set( "sql.trace.txn.enable_threshold","跟踪所有事务的持续时间（设置为0以禁用）")
mapDec.set("timeseries.storage.enabled","如果设置，则定期时间序列数据存储在集群中； 除非您将数据存储在其他位置，否则不建议禁用");
mapDec.set("timeseries.storage.resolution_10s.ttl","以10秒分辨率存储的时间序列数据的最大寿命。 早于此的数据将被汇总和删除。");
mapDec.set("timeseries.storage.resolution_30m.ttl","以30分钟的分辨率存储的时间序列数据的最长使用期限。 早于此的数据将被删除。");
mapDec.set("trace.debug.enable","如果已设置，则可以在/ debug页面中查看最近请求的跟踪");
mapDec.set("trace.lightstep.token","如果已设置，则使用此令牌将痕迹转到Lightstep");
mapDec.set("trace.zipkin.collector","如果已设置，则跟踪将转到给定的Zipkin实例（示例：“ 127.0.0.1:9411”）； 如果设置了trace.lightstep.token，则忽略");
mapDec.set("version","集群版本");
export default mapDec