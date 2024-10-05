local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local now = tonumber(ARGV[3])

local windowStart = now - window

local len = tonumber(redis.call('LLEN', key))

if(len >= limit) then
    local head = tonumber(redis.call('LPOP', key))
    while head <= windowStart do
        head = tonumber(redis.call('LPOP', key))
    end
    redis.call('LPUSH', key, head)
    len = tonumber(redis.call('LLEN', key))
end

if(len < limit) then
    redis.call('RPUSH', key, now)
    redis.call('PEXPIRE', key, window)
    return 1
end
return 0