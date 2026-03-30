#version 330

// Input vertex attributes (from vertex shader)
in vec2 fragTexCoord;
in vec4 fragColor;

// Input uniform values
uniform sampler2D texture0;
uniform vec4 colDiffuse;

// Output fragment color
out vec4 finalColor;

// NOTE: Add your custom variables here
in vec2 vRectPos;
uniform vec2 uRectSize;
uniform float[4] uCornerRadius;
uniform float[4] uBorderWidth;
uniform vec4 uBorderColor;
uniform vec4 uGradientStartColor;
uniform vec4 uGradientEndColor;
uniform float uGradientAngle;
uniform int uRenderMode;

#define RENDER_MODE_SOLID_COLOR     0
#define RENDER_MODE_GRADIENT_LINEAR 1
#define RENDER_MODE_GRADIENT_RADIAL 2
#define RENDER_MODE_TEXTURE         3

float fastGreaterThanDistance(vec2 p0, vec2 p1, float dist) {
    lowp vec2 diff = p1 - p0;
    return float(dot(diff, diff) > dist*dist); 
}

float cornerSDF(vec2 p0, vec2 p1, float dist) {
    return distance(p0, p1) - dist;
}

void main() {
    vec4 texelColor;
    switch (uRenderMode) {
        case RENDER_MODE_SOLID_COLOR:
            texelColor = fragColor;
            break;

        case RENDER_MODE_GRADIENT_LINEAR:
            // rotate clockwise
            vec2 dir = vec2(cos(-uGradientAngle), sin(-uGradientAngle));
            float aspect = uRectSize.x / uRectSize.y;
            vec2 pos = fragTexCoord - 0.5;
            pos.x *= aspect;
            float projection = dot(pos, dir);
            float maxProjection = (abs(dir.x * aspect) + abs(dir.y)) * 0.5;
            float t = (projection / maxProjection + 1) / 2;
            texelColor = mix(uGradientStartColor, uGradientEndColor, t);
            break;

        // case RENDER_MODE_GRADIENT_RADIAL:
        //     break;

        case RENDER_MODE_TEXTURE:
            texelColor = texture(texture0, fragTexCoord) * fragColor;
            break;

        default:
            texelColor = vec4(1,0,1,1);
    }

    lowp float isCorner[4] = float[](
        float(vRectPos.x < uCornerRadius[0]               && vRectPos.y < uCornerRadius[0]),
        float(vRectPos.x > uRectSize.x - uCornerRadius[1] && vRectPos.y < uCornerRadius[1]),
        float(vRectPos.x > uRectSize.x - uCornerRadius[2] && vRectPos.y > uRectSize.y - uCornerRadius[2]),
        float(vRectPos.x < uCornerRadius[3]               && vRectPos.y > uRectSize.y - uCornerRadius[3])
    );

    vec4 alphaMask = vec4(1, 1, 1, 1);

    lowp float sdf = -1;
    if        (isCorner[0] == 1) {
        sdf = cornerSDF(vRectPos, vec2(uCornerRadius[0], uCornerRadius[0]), uCornerRadius[0]);
    } else if (isCorner[1] == 1) {
        sdf = cornerSDF(vRectPos, vec2(uRectSize.x - uCornerRadius[1], uCornerRadius[1]), uCornerRadius[1]);
    } else if (isCorner[2] == 1) {
        sdf = cornerSDF(vRectPos, vec2(uRectSize.x - uCornerRadius[2], uRectSize.y - uCornerRadius[2]), uCornerRadius[2]);
    } else if (isCorner[3] == 1) {
        sdf = cornerSDF(vRectPos, vec2(uCornerRadius[3], uRectSize.y - uCornerRadius[3]), uCornerRadius[3]);
    }
    alphaMask.a = smoothstep(1, -1, sdf);

    float[4] innerRadius = float[](
        uCornerRadius[0] - uBorderWidth[0],
        uCornerRadius[1] - uBorderWidth[1],
        uCornerRadius[2] - uBorderWidth[2],
        uCornerRadius[3] - uBorderWidth[3]
    );

    lowp float isCornerBorder = float(
        isCorner[0] * fastGreaterThanDistance(vRectPos, vec2(uCornerRadius[0], uCornerRadius[0]), innerRadius[0]) +
        isCorner[1] * fastGreaterThanDistance(vRectPos, vec2(uRectSize.x - uCornerRadius[1], uCornerRadius[1]), innerRadius[1]) +
        isCorner[2] * fastGreaterThanDistance(vRectPos, vec2(uRectSize.x - uCornerRadius[2], uRectSize.y - uCornerRadius[2]), innerRadius[2]) +
        isCorner[3] * fastGreaterThanDistance(vRectPos, vec2(uCornerRadius[3], uRectSize.y - uCornerRadius[3]), innerRadius[3]) > 0
    );

    lowp float isBorder = float(
        vRectPos.x < uBorderWidth[3] || vRectPos.y < uBorderWidth[0] || vRectPos.x > uRectSize.x - uBorderWidth[1] || vRectPos.y > uRectSize.y - uBorderWidth[2] ||
        isCornerBorder == 1
    );

    vec4 borderColor = uBorderColor * isBorder;

    finalColor = vec4(
        borderColor.rgb * borderColor.a + texelColor.rgb * texelColor.a * (1 - borderColor.a),
        borderColor.a + texelColor.a * (1 - borderColor.a)
    ) * alphaMask;
}
