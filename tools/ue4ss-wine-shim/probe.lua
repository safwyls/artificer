-- dwbridge feasibility probe: can a Lua mod see live game objects?
print("[DwbridgeProbe] alive inside the server\n")
ExecuteWithDelay(15000, function()
    local ok, err = pcall(function()
        local world = FindFirstOf("World")
        print(string.format("[DwbridgeProbe] World -> %s\n", world and world:GetFullName() or "nil"))
        local gm = FindFirstOf("GameModeBase")
        print(string.format("[DwbridgeProbe] GameMode -> %s\n", gm and gm:GetFullName() or "nil"))
    end)
    if not ok then print("[DwbridgeProbe] error: " .. tostring(err) .. "\n") end
end)
