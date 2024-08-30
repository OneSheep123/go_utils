val = redis.call('get', KEYS[1])
-- 注意:redis返回类型转lua脚本类型
if val == false then
    -- 锁不存在
    return redis.call('set', KEYS[1], ARGV[1], 'nx', ARGV[2])
elseif val == ARGV[1] then
    -- 锁存在，且当前是加自己的锁
    redis.call('expire', KEYS[1], ARGV[2])
    return 'OK'
else
    -- 锁被别人拿着
    return ''
end