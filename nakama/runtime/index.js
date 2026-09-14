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

function profileString(value, field) {
  if (value === null || value === undefined) {
    return null;
  }
  if (typeof value !== "string" || value.length > 128) {
    throw { message: field + " must be a string of at most 128 characters", code: 3 };
  }
  return value;
}

function gameserviceProfile(ctx, logger, nk, payload) {
  if (!ctx.userId) {
    throw { message: "profile RPC requires an authenticated user", code:  unauthenticatedCode() };
  }
  var request = {};
  if (payload) {
    try {
      request = JSON.parse(payload);
    } catch (error) {
      throw { message: "profile payload must be valid JSON", code: 3 };
    }
  }
  if (request.operation && request.operation !== "get" && request.operation !== "patch_public_fields") {
    throw { message: "unsupported profile operation", code: 3 };
  }
  if (request.operation === "patch_public_fields") {
    var fields = request.fields || {};
    if (typeof fields !== "object" || Array.isArray(fields)) {
      throw { message: "profile fields must be an object", code: 3 };
    }
    var allowedFields = { username: true, displayName: true, langTag: true, avatarUrl: true };
    Object.keys(fields).forEach(function (field) {
      if (!allowedFields[field]) {
        throw { message: "unsupported profile field", code: 3 };
      }
    });
    nk.accountUpdateId(
      ctx.userId,
      profileString(fields.username, "username"),
      profileString(fields.displayName, "displayName"),
      null,
      null,
      profileString(fields.langTag, "langTag"),
      profileString(fields.avatarUrl, "avatarUrl"),
      {}
    );
  }
  var account = nk.accountGetId(ctx.userId);
  return JSON.stringify({
    userId: ctx.userId,
    username: account.user.username || "",
    displayName: account.user.displayName || "",
    langTag: account.user.langTag || "",
    avatarUrl: account.user.avatarUrl || "",
    metadata: account.user.metadata || {}
  });
}

function unauthenticatedCode() {
  return 16;
}

function InitModule(ctx, logger, nk, initializer) {
  initializer.registerRpc("gameservice.health", gameserviceHealth);
  initializer.registerRpc("gameservice.profile", gameserviceProfile);
  logger.info("GameService Nakama bridge loaded.");
}
