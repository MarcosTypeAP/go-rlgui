#version 330

// Input vertex attributes
in vec3 vertexPosition;
in vec2 vertexTexCoord;
in vec3 vertexNormal;
in vec4 vertexColor;

// Input uniform values
uniform mat4 mvp;

// Output vertex attributes (to fragment shader)
out vec2 fragTexCoord;
out vec4 fragColor;

// NOTE: Add your custom variables here
out vec2 vRectPos;
uniform vec2 uRectTopLeft;

void main() {
    // Send vertex attributes to fragment shader
    fragTexCoord = vertexTexCoord;
    fragColor = vertexColor;

    vRectPos = vertexPosition.xy - uRectTopLeft;

    // Calculate final vertex position
    gl_Position = mvp*vec4(vertexPosition, 1);
}
