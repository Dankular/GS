// GameService's Nakama bridge. Nakama executes this file in its sandboxed
// JavaScript runtime; it deliberately has no filesystem, SQL, or secret access.
function gameserviceHealth(ctx, logger, nk, payload) {
  var response = nk.httpRequest(
    "http://control-api:8080/health/live",
    "get",
    { "Accept": "application/json" },
    "",
    2000
  );
  if (!response || response.code !== 200) {
    throw { message: "GameService control plane is unavailable", code: 14 };
  }
  return JSON.stringify({
    nakama: true,
    controlPlane: true,
    userId: ctx.userId || ""
  });
}

function InitModule(ctx, logger, nk, initializer) {
  initializer.registerRpc("gameservice.health", gameserviceHealth);
  logger.info("GameService Nakama bridge loaded.");
}
