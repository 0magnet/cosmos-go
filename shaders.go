//go:build js && wasm

package cosmos

import (
	"fmt"
	"strings"
)

// All shaders are carried over verbatim from cosmos 1.6.1.

const quadVert = `#ifdef GL_ES
precision highp float;
#endif

attribute vec2 quad;
varying vec2 index;

void main() {
    index = (quad + 1.0) / 2.0;
    gl_Position = vec4(quad, 0, 1);
}`

const clearFrag = `#ifdef GL_ES
precision highp float;
#endif

varying vec2 index;

void main() {
  gl_FragColor = vec4(0.0);
}`

const updatePositionFrag = `#ifdef GL_ES
precision highp float;
#endif

uniform sampler2D position;
uniform sampler2D velocity;
uniform float friction;
uniform float spaceSize;

varying vec2 index;

void main() {
  vec4 pointPosition = texture2D(position, index);
  vec4 pointVelocity = texture2D(velocity, index);

  // Friction
  pointVelocity.rg *= friction;

  pointPosition.rg += pointVelocity.rg;

  pointPosition.r = clamp(pointPosition.r, 0.0, spaceSize);
  pointPosition.g = clamp(pointPosition.g, 0.0, spaceSize);

  gl_FragColor = pointPosition;
}`

const drawPointsVert = `#ifdef GL_ES
precision highp float;
#endif

attribute vec2 indexes;

uniform sampler2D positions;
uniform sampler2D particleColor;
uniform sampler2D particleGreyoutStatus;
uniform sampler2D particleSize;
uniform float ratio;
uniform mat3 transform;
uniform float pointsTextureSize;
uniform float sizeScale;
uniform float spaceSize;
uniform vec2 screenSize;
uniform float greyoutOpacity;
uniform bool scaleNodesOnZoom;
uniform float maxPointSize;

varying vec2 index;
varying vec3 rgbColor;
varying float alpha;

float pointSize(float size) {
  float pSize;
  if (scaleNodesOnZoom) {
    pSize = size * ratio * transform[0][0];
  } else {
    pSize = size * ratio * min(5.0, max(1.0, transform[0][0] * 0.01));
  }

  return min(pSize, maxPointSize * ratio);
}

void main() {
  index = indexes;
  // Position
  vec4 pointPosition = texture2D(positions, (index + 0.5) / pointsTextureSize);
  vec2 point = pointPosition.rg;
  vec2 p = 2.0 * point / spaceSize - 1.0;
  p *= spaceSize / screenSize;
  vec3 final = transform * vec3(p, 1);
  gl_Position = vec4(final.rg, 0, 1);

  // Size
  vec4 pSize = texture2D(particleSize, (index + 0.5) / pointsTextureSize);
  float size = pSize.r * sizeScale;

  // Color
  vec4 pColor = texture2D(particleColor, (index + 0.5) / pointsTextureSize);
  rgbColor = pColor.rgb;
  gl_PointSize = pointSize(size);

  alpha = pColor.a;
  // Alpha of selected points
  vec4 greyoutStatus = texture2D(particleGreyoutStatus, (index + 0.5) / pointsTextureSize);
  if (greyoutStatus.r > 0.0) {
    alpha *= greyoutOpacity;
  }
}`

const drawPointsFrag = `#ifdef GL_ES
precision highp float;
#endif

varying vec2 index;
varying vec3 rgbColor;
varying float alpha;

const float smoothing = 0.9;

void main() {
    if (alpha == 0.0) {
        discard;
    }
    float r = 0.0;
    float delta = 0.0;
    vec2 cxy = 2.0 * gl_PointCoord - 1.0;
    r = dot(cxy, cxy);
    float opacity = alpha * (1.0 - smoothstep(smoothing, 1.0, r));

    gl_FragColor = vec4(rgbColor, opacity);
}`

const drawHighlightedVert = `precision mediump float;

attribute vec2 quad;

uniform sampler2D positions;
uniform sampler2D particleColor;
uniform sampler2D particleGreyoutStatus;
uniform sampler2D particleSize;
uniform mat3 transform;
uniform float pointsTextureSize;
uniform float sizeScale;
uniform float spaceSize;
uniform vec2 screenSize;
uniform bool scaleNodesOnZoom;
uniform float pointIndex;
uniform float maxPointSize;
uniform vec4 color;
uniform float greyoutOpacity;

varying vec2 pos;
varying float particleOpacity;

float pointSize(float size) {
  float pSize;
  if (scaleNodesOnZoom) {
    pSize = size * transform[0][0];
  } else {
    pSize = size * min(5.0, max(1.0, transform[0][0] * 0.01));
  }
  return min(pSize, maxPointSize);
}

const float relativeRingRadius = 1.3;

void main () {
  pos = quad;

  vec2 ij = vec2(mod(pointIndex, pointsTextureSize), floor(pointIndex / pointsTextureSize)) + 0.5;
  vec4 pointPosition = texture2D(positions, ij / pointsTextureSize);
  vec4 pSize = texture2D(particleSize, ij / pointsTextureSize);
  vec4 pColor = texture2D(particleColor, ij / pointsTextureSize);
  particleOpacity = pColor.a;
  // Alpha of selected points
  vec4 greyoutStatus = texture2D(particleGreyoutStatus, ij / pointsTextureSize);
  if (greyoutStatus.r > 0.0) {
    particleOpacity *= greyoutOpacity;
  }
  float size = (pointSize(pSize.r * sizeScale) * relativeRingRadius) / transform[0][0];
  float radius = size * 0.5;
  vec2 a = pointPosition.xy;
  vec2 b = pointPosition.xy + vec2(0.0, radius);
  vec2 xBasis = b - a;
  vec2 yBasis = normalize(vec2(-xBasis.y, xBasis.x));
  vec2 point = a + xBasis * quad.x + yBasis * radius * quad.y;
  vec2 p = 2.0 * point / spaceSize - 1.0;
  p *= spaceSize / screenSize;
  vec3 final =  transform * vec3(p, 1);

  gl_Position = vec4(final.rg, 0, 1);
}`

const drawHighlightedFrag = `precision mediump float;

uniform vec4 color;
uniform float width;

varying vec2 pos;
varying float particleOpacity;

const float smoothing = 1.05;

void main () {
  vec2 cxy = pos;
  float r = dot(cxy, cxy);
  float opacity = smoothstep(r, r * smoothing, 1.0);
  float stroke = smoothstep(width, width * smoothing, r);
  gl_FragColor = vec4(color.rgb, opacity * stroke * color.a * particleOpacity);
}`

const findPointsOnAreaSelectionFrag = `#ifdef GL_ES
precision highp float;
#endif

uniform sampler2D position;
uniform sampler2D particleSize;
uniform float sizeScale;
uniform float spaceSize;
uniform vec2 screenSize;
uniform float ratio;
uniform mat3 transform;
uniform vec2 selection[2];
uniform bool scaleNodesOnZoom;
uniform float maxPointSize;

varying vec2 index;

float pointSize(float size) {
  float pSize;
  if (scaleNodesOnZoom) {
    pSize = size * ratio * transform[0][0];
  } else {
    pSize = size * ratio * min(5.0, max(1.0, transform[0][0] * 0.01));
  }
  return min(pSize, maxPointSize * ratio);
}

void main() {
  vec4 pointPosition = texture2D(position, index);
  vec2 p = 2.0 * pointPosition.rg / spaceSize - 1.0;
  p *= spaceSize / screenSize;
  vec3 final = transform * vec3(p, 1);

  vec4 pSize = texture2D(particleSize, index);
  float size = pSize.r * sizeScale;

  float left = 2.0 * (selection[0].x - 0.5 * pointSize(size)) / screenSize.x - 1.0;
  float right = 2.0 * (selection[1].x + 0.5 * pointSize(size)) / screenSize.x - 1.0;
  float top =  2.0 * (selection[0].y - 0.5 * pointSize(size)) / screenSize.y - 1.0;
  float bottom =  2.0 * (selection[1].y + 0.5 * pointSize(size)) / screenSize.y - 1.0;

  gl_FragColor = vec4(0.0, 0.0, pointPosition.rg);
  if (final.x >= left && final.x <= right && final.y >= top && final.y <= bottom) {
    gl_FragColor.r = 1.0;
  }
}`

const findHoveredPointVert = `#ifdef GL_ES
precision highp float;
#endif

uniform sampler2D position;
uniform float pointsTextureSize;
uniform sampler2D particleSize;
uniform float sizeScale;
uniform float spaceSize;
uniform vec2 screenSize;
uniform float ratio;
uniform mat3 transform;
uniform vec2 mousePosition;
uniform bool scaleNodesOnZoom;
uniform float maxPointSize;

attribute vec2 indexes;

varying vec4 rgba;

float pointSize(float size) {
  float pSize;
  if (scaleNodesOnZoom) {
    pSize = size * ratio * transform[0][0];
  } else {
    pSize = size * ratio * min(5.0, max(1.0, transform[0][0] * 0.01));
  }
  return min(pSize, maxPointSize * ratio);
}

float euclideanDistance (float x1, float x2, float y1, float y2) {
  return sqrt((x2 - x1) * (x2 - x1) + (y2 - y1) * (y2 - y1));
}

void main() {
  vec4 pointPosition = texture2D(position, (indexes + 0.5) / pointsTextureSize);
  vec2 p = 2.0 * pointPosition.rg / spaceSize - 1.0;
  p *= spaceSize / screenSize;
  vec3 final = transform * vec3(p, 1);

  vec4 pSize = texture2D(particleSize, indexes / pointsTextureSize);
  float size = pSize.r * sizeScale;
  float pointRadius = 0.5 * pointSize(size);

  vec2 pointScreenPosition = (final.xy + 1.0) * screenSize / 2.0;
  rgba = vec4(0.0);
  gl_Position = vec4(0.5, 0.5, 0.0, 1.0);
  if (euclideanDistance(pointScreenPosition.x, mousePosition.x, pointScreenPosition.y, mousePosition.y) < pointRadius / ratio) {
    float index = indexes.g * pointsTextureSize + indexes.r;
    rgba = vec4(index, pSize.r, pointPosition.xy);
    gl_Position = vec4(-0.5, -0.5, 0.0, 1.0);
  }

  gl_PointSize = 1.0;
}`

const findHoveredPointFrag = `#ifdef GL_ES
precision highp float;
#endif

varying vec4 rgba;

void main() {
  gl_FragColor = rgba;
}`

const fillSampledNodesVert = `#ifdef GL_ES
precision highp float;
#endif

uniform sampler2D position;
uniform float pointsTextureSize;
uniform float spaceSize;
uniform vec2 screenSize;
uniform mat3 transform;

attribute vec2 indexes;

varying vec4 rgba;

void main() {
  vec4 pointPosition = texture2D(position, (indexes + 0.5) / pointsTextureSize);
  vec2 p = 2.0 * pointPosition.rg / spaceSize - 1.0;
  p *= spaceSize / screenSize;
  vec3 final = transform * vec3(p, 1);

  vec2 pointScreenPosition = (final.xy + 1.0) * screenSize / 2.0;
  float index = indexes.g * pointsTextureSize + indexes.r;
  rgba = vec4(index, 1.0, pointPosition.xy);
  float i = (pointScreenPosition.x + 0.5) / screenSize.x;
  float j = (pointScreenPosition.y + 0.5) / screenSize.y;
  gl_Position = vec4(2.0 * vec2(i, j) - 1.0, 0.0, 1.0);

  gl_PointSize = 1.0;
}`

const fillSampledNodesFrag = `#ifdef GL_ES
precision highp float;
#endif

varying vec4 rgba;

void main() {
  gl_FragColor = rgba;
}`

const trackPositionsFrag = `#ifdef GL_ES
precision highp float;
#endif

uniform sampler2D position;
uniform sampler2D trackedIndices;
uniform float pointsTextureSize;

varying vec2 index;

void main() {
  vec4 trackedPointIndicies = texture2D(trackedIndices, index);
  if (trackedPointIndicies.r < 0.0) discard;
  vec4 pointPosition = texture2D(position, (trackedPointIndicies.rg + 0.5) / pointsTextureSize);

  gl_FragColor = vec4(pointPosition.rg, 1.0, 1.0);
}`

const drawLineVert = `precision highp float;
attribute vec2 position, pointA, pointB;
attribute vec4 color;
attribute float width;
attribute float arrow;
uniform sampler2D positions;
uniform sampler2D particleGreyoutStatus;
uniform mat3 transform;
uniform float pointsTextureSize;
uniform float widthScale;
uniform float nodeSizeScale;
uniform float arrowSizeScale;
uniform float spaceSize;
uniform vec2 screenSize;
uniform float ratio;
uniform vec2 linkVisibilityDistanceRange;
uniform float linkVisibilityMinTransparency;
uniform float greyoutOpacity;
uniform bool scaleNodesOnZoom;
uniform float curvedWeight;
uniform float curvedLinkControlPointDistance;
uniform float curvedLinkSegments;

varying vec4 rgbaColor;
varying vec2 pos;
varying float arrowLength;
varying float linkWidthArrowWidthRatio;
varying float smoothWidthRatio;
varying float useArrow;

float map(float value, float min1, float max1, float min2, float max2) {
  return min2 + (value - min1) * (max2 - min2) / (max1 - min1);
}

vec2 conicParametricCurve(vec2 A, vec2 B, vec2 ControlPoint, float t, float w) {
  vec2 divident = (1.0 - t) * (1.0 - t) * A + 2.0 * (1.0 - t) * t * w * ControlPoint + t * t * B;
  float divisor = (1.0 - t) * (1.0 - t) + 2.0 * (1.0 - t) * t * w + t * t;
  return divident / divisor;
}

void main() {
  pos = position;

  vec2 pointTexturePosA = (pointA + 0.5) / pointsTextureSize;
  vec2 pointTexturePosB = (pointB + 0.5) / pointsTextureSize;
  // Greyed out status of points
  vec4 greyoutStatusA = texture2D(particleGreyoutStatus, pointTexturePosA);
  vec4 greyoutStatusB = texture2D(particleGreyoutStatus, pointTexturePosB);
  // Position
  vec4 pointPositionA = texture2D(positions, pointTexturePosA);
  vec4 pointPositionB = texture2D(positions, pointTexturePosB);
  vec2 a = pointPositionA.xy;
  vec2 b = pointPositionB.xy;
  vec2 xBasis = b - a;
  vec2 yBasis = normalize(vec2(-xBasis.y, xBasis.x));

  // Calculate link distance
  float linkDist = length(xBasis);
  float h = curvedLinkControlPointDistance;
  vec2 controlPoint = (a + b) / 2.0 + yBasis * linkDist * h;

  float linkDistPx = linkDist * transform[0][0];

  float linkWidth = width * widthScale;
  float k = 2.0;
  float arrowWidth = max(5.0, linkWidth * k);
  arrowWidth *= arrowSizeScale;

  float arrowWidthPx = arrowWidth / transform[0][0];
  arrowLength = min(0.3, (0.866 * arrowWidthPx * 2.0) / linkDist);

  float smoothWidth = 2.0;
  float arrowExtraWidth = arrowWidth - linkWidth;
  linkWidth += smoothWidth / 2.0;
  useArrow = arrow;
  if (useArrow > 0.5) {
    linkWidth += arrowExtraWidth;
  }
  smoothWidthRatio = smoothWidth / linkWidth;
  linkWidthArrowWidthRatio = arrowExtraWidth / linkWidth;

  float linkWidthPx = linkWidth / transform[0][0];

  // Color
  vec3 rgbColor = color.rgb;
  float opacity = color.a * max(linkVisibilityMinTransparency, map(linkDistPx, linkVisibilityDistanceRange.g, linkVisibilityDistanceRange.r, 0.0, 1.0));

  if (greyoutStatusA.r > 0.0 || greyoutStatusB.r > 0.0) {
    opacity *= greyoutOpacity;
  }

  rgbaColor = vec4(rgbColor, opacity);

  float t = position.x;
  float w = curvedWeight;
  float tPrev = t - 1.0 / curvedLinkSegments;
  float tNext = t + 1.0 / curvedLinkSegments;
  vec2 pointCurr = conicParametricCurve(a, b, controlPoint, t, w);
  vec2 pointPrev = conicParametricCurve(a, b, controlPoint, max(0.0, tPrev), w);
  vec2 pointNext = conicParametricCurve(a, b, controlPoint, min(tNext, 1.0), w);
  vec2 xBasisCurved = pointNext - pointPrev;
  vec2 yBasisCurved = normalize(vec2(-xBasisCurved.y, xBasisCurved.x));
  pointCurr += yBasisCurved * linkWidthPx * position.y;
  vec2 p = 2.0 * pointCurr / spaceSize - 1.0;
  p *= spaceSize / screenSize;
  vec3 final =  transform * vec3(p, 1);
  gl_Position = vec4(final.rg, 0, 1);
}`

const drawLineFrag = `precision highp float;

varying vec4 rgbaColor;
varying vec2 pos;
varying float arrowLength;
varying float linkWidthArrowWidthRatio;
varying float smoothWidthRatio;
varying float useArrow;

float map(float value, float min1, float max1, float min2, float max2) {
  return min2 + (value - min1) * (max2 - min2) / (max1 - min1);
}

void main() {
  float opacity = 1.0;
  vec3 color = rgbaColor.rgb;
  float smoothDelta = smoothWidthRatio / 2.0;
  if (useArrow > 0.5) {
    float end_arrow = 0.5 + arrowLength / 2.0;
    float start_arrow = end_arrow - arrowLength;
    float arrowWidthDelta = linkWidthArrowWidthRatio / 2.0;
    float linkOpacity = rgbaColor.a * smoothstep(0.5 - arrowWidthDelta, 0.5 - arrowWidthDelta - smoothDelta, abs(pos.y));
    float arrowOpacity = 1.0;
    if (pos.x > start_arrow && pos.x < start_arrow + arrowLength) {
      float xmapped = map(pos.x, start_arrow, end_arrow, 0.0, 1.0);
      arrowOpacity = rgbaColor.a * smoothstep(xmapped - smoothDelta, xmapped, map(abs(pos.y), 0.5, 0.0, 0.0, 1.0));
      if (linkOpacity != arrowOpacity) {
        linkOpacity += arrowOpacity;
      }
    }
    opacity = linkOpacity;
  } else opacity = rgbaColor.a * smoothstep(0.5, 0.5 - smoothDelta, abs(pos.y));

  gl_FragColor = vec4(color, opacity);
}`

const forceGravityFrag = `#ifdef GL_ES
precision highp float;
#endif

uniform sampler2D position;
uniform float gravity;
uniform float spaceSize;
uniform float alpha;

varying vec2 index;

void main() {
  vec4 pointPosition = texture2D(position, index);
  vec4 velocity = vec4(0.0);
  vec2 centerPosition = vec2(spaceSize / 2.0);
  vec2 distVector = centerPosition - pointPosition.rg;
  float dist = sqrt(dot(distVector, distVector));
  if (dist > 0.0) {
    float angle = atan(distVector.y, distVector.x);
    float addV = alpha * gravity * dist * 0.1;
    velocity.rg += addV * vec2(cos(angle), sin(angle));
  }

  gl_FragColor = velocity;
}`

const calculateCentermassVert = `#ifdef GL_ES
precision highp float;
#endif

uniform sampler2D position;
uniform float pointsTextureSize;

attribute vec2 indexes;

varying vec4 rgba;

void main() {
  vec4 pointPosition = texture2D(position, indexes / pointsTextureSize);
  rgba = vec4(pointPosition.xy, 1.0, 0.0);

  gl_Position = vec4(0.0, 0.0, 0.0, 1.0);
  gl_PointSize = 1.0;
}`

const calculateCentermassFrag = `#ifdef GL_ES
precision highp float;
#endif

varying vec4 rgba;

void main() {
  gl_FragColor = rgba;
}`

const forceCenterFrag = `#ifdef GL_ES
precision highp float;
#endif

uniform sampler2D position;
uniform sampler2D centermass;
uniform float center;
uniform float alpha;

varying vec2 index;


void main() {
  vec4 pointPosition = texture2D(position, index);
  vec4 velocity = vec4(0.0);
  vec4 centermassValues = texture2D(centermass, vec2(0.0));
  vec2 centermassPosition = centermassValues.xy / centermassValues.b;
  vec2 distVector = centermassPosition - pointPosition.xy;
  float dist = sqrt(dot(distVector, distVector));
  if (dist > 0.0) {
    float angle = atan(distVector.y, distVector.x);
    float addV = alpha * center * dist * 0.01;
    velocity.rg += addV * vec2(cos(angle), sin(angle));
  }

  gl_FragColor = velocity;
}`

const forceMouseFrag = `#ifdef GL_ES
precision highp float;
#endif

uniform sampler2D position;
uniform float repulsion;
uniform vec2 mousePos;

varying vec2 index;

void main() {
  vec4 pointPosition = texture2D(position, index);
  vec4 velocity = vec4(0.0);
  vec2 mouse = mousePos;
  // Move particles from mouse position
  vec2 distVector = mouse - pointPosition.rg;
  float dist = sqrt(dot(distVector, distVector));
  dist = max(dist, 10.0);
  float angle = atan(distVector.y, distVector.x);
  float addV = 100.0 * repulsion / (dist * dist);
  velocity.rg -= addV * vec2(cos(angle), sin(angle));

  gl_FragColor = velocity;
}`

const calculateLevelVert = `#ifdef GL_ES
precision highp float;
#endif

uniform sampler2D position;
uniform float pointsTextureSize;
uniform float levelTextureSize;
uniform float cellSize;

attribute vec2 indexes;

varying vec4 rgba;

void main() {
  vec4 pointPosition = texture2D(position, indexes / pointsTextureSize);
  rgba = vec4(pointPosition.rg, 1.0, 0.0);

  float n = floor(pointPosition.x / cellSize);
  float m = floor(pointPosition.y / cellSize);

  vec2 levelPosition = 2.0 * (vec2(n, m) + 0.5) / levelTextureSize - 1.0;

  gl_Position = vec4(levelPosition, 0.0, 1.0);
  gl_PointSize = 1.0;
}`

const calculateLevelFrag = `#ifdef GL_ES
precision highp float;
#endif

varying vec4 rgba;

void main() {
  gl_FragColor = rgba;
}`

const forceLevelFrag = `#ifdef GL_ES
precision highp float;
#endif

uniform sampler2D position;
uniform sampler2D levelFbo;

uniform float level;
uniform float levels;
uniform float levelTextureSize;
uniform float repulsion;
uniform float alpha;
uniform float spaceSize;
uniform float theta;

varying vec2 index;

const float MAX_LEVELS_NUM = 14.0;

vec2 calcAdd (vec2 ij, vec2 pp) {
  vec2 add = vec2(0.0);
  vec4 centermass = texture2D(levelFbo, ij);
  if (centermass.r > 0.0 && centermass.g > 0.0 && centermass.b > 0.0) {
    vec2 centermassPosition = vec2(centermass.rg / centermass.b);
    vec2 distVector = pp - centermassPosition;
    float l = dot(distVector, distVector);
    float dist = sqrt(l);
    if (l > 0.0) {
      float angle = atan(distVector.y, distVector.x);
      float c = alpha * repulsion * centermass.b;

      float distanceMin2 = 1.0;
      if (l < distanceMin2) l = sqrt(distanceMin2 * l);
      float addV = c / sqrt(l);
      add = addV * vec2(cos(angle), sin(angle));
    }
  }
  return add;
}

void main() {
  vec4 pointPosition = texture2D(position, index);
  float x = pointPosition.x;
  float y = pointPosition.y;

  float left = 0.0;
  float top = 0.0;
  float right = spaceSize;
  float bottom = spaceSize;

  float n_left = 0.0;
  float n_top = 0.0;
  float n_right = 0.0;
  float n_bottom = 0.0;

  float cellSize = 0.0;

  for (float i = 0.0; i < MAX_LEVELS_NUM; i += 1.0) {
    if (i <= level) {
      left += cellSize * n_left;
      top += cellSize * n_top;
      right -= cellSize * n_right;
      bottom -= cellSize * n_bottom;

      cellSize = pow(2.0 , levels - i - 1.0);

      float dist_left = x - left;
      n_left = max(0.0, floor(dist_left / cellSize - theta));

      float dist_top = y - top;
      n_top = max(0.0, floor(dist_top / cellSize - theta));

      float dist_right = right - x;
      n_right = max(0.0, floor(dist_right / cellSize - theta));

      float dist_bottom = bottom - y;
      n_bottom = max(0.0, floor(dist_bottom / cellSize - theta));

    }
  }

  vec4 velocity = vec4(vec2(0.0), 1.0, 0.0);

  for (float i = 0.0; i < 12.0; i += 1.0) {
    for (float j = 0.0; j < 4.0; j += 1.0) {
      float n = left + cellSize * j;
      float m = top + cellSize * n_top + cellSize * i;

      if (n < (left + n_left * cellSize) && m < bottom) {
        velocity.xy += calcAdd(vec2(n / cellSize, m / cellSize) / levelTextureSize, pointPosition.xy);
      }

      n = left + cellSize * i;
      m = top + cellSize * j;

      if (n < (right - n_right * cellSize) && m < (top + n_top * cellSize)) {
        velocity.xy += calcAdd(vec2(n / cellSize, m / cellSize) / levelTextureSize, pointPosition.xy);
      }

      n = right - n_right * cellSize + cellSize * j;
      m = top + cellSize * i;

      if (n < right && m < (bottom - n_bottom * cellSize)) {
        velocity.xy += calcAdd(vec2(n / cellSize, m / cellSize) / levelTextureSize, pointPosition.xy);
      }

      n = left + n_left * cellSize + cellSize * i;
      m = bottom - n_bottom * cellSize + cellSize * j;

      if (n < right && m < bottom) {
        velocity.xy += calcAdd(vec2(n / cellSize, m / cellSize) / levelTextureSize, pointPosition.xy);
      }
    }
  }

  gl_FragColor = velocity;
}`

const forceCentermassFrag = `#ifdef GL_ES
precision highp float;
#endif

uniform sampler2D position;
uniform sampler2D levelFbo;
uniform sampler2D randomValues;

uniform float levelTextureSize;
uniform float repulsion;
uniform float alpha;

varying vec2 index;

vec2 calcAdd (vec2 ij, vec2 pp) {
  vec2 add = vec2(0.0);
  vec4 centermass = texture2D(levelFbo, ij);
  if (centermass.r > 0.0 && centermass.g > 0.0 && centermass.b > 0.0) {
    vec2 centermassPosition = vec2(centermass.rg / centermass.b);
    vec2 distVector = pp - centermassPosition;
    float l = dot(distVector, distVector);
    float dist = sqrt(l);
    if (l > 0.0) {
      float angle = atan(distVector.y, distVector.x);
      float c = alpha * repulsion * centermass.b;

      float distanceMin2 = 1.0;
      if (l < distanceMin2) l = sqrt(distanceMin2 * l);
      float addV = c / sqrt(l);
      add = addV * vec2(cos(angle), sin(angle));
    }
  }
  return add;
}

void main() {
  vec4 pointPosition = texture2D(position, index);
  vec4 random = texture2D(randomValues, index);

  vec4 velocity = vec4(0.0);
  velocity.xy += calcAdd(pointPosition.xy / levelTextureSize, pointPosition.xy);
  velocity.xy += velocity.xy * random.rg;

  gl_FragColor = velocity;
}`

// forceSpringFrag generates the ForceLink shader; maxLinks is inlined as a
// loop bound (WebGL 1 requires constant loop bounds), mirroring
// ForceLink/force-spring.ts.
func forceSpringFrag(maxLinks int) string {
	return fmt.Sprintf(`
#ifdef GL_ES
precision highp float;
#endif

uniform sampler2D position;
uniform float linkSpring;
uniform float linkDistance;
uniform vec2 linkDistRandomVariationRange;

uniform sampler2D linkFirstIndicesAndAmount;
uniform sampler2D linkIndices;
uniform sampler2D linkBiasAndStrength;
uniform sampler2D linkRandomDistanceFbo;

uniform float pointsTextureSize;
uniform float linksTextureSize;
uniform float alpha;

varying vec2 index;

const float MAX_LINKS = %d.0;

void main() {
  vec4 pointPosition = texture2D(position, index);
  vec4 velocity = vec4(0.0);

  vec4 linkFirstIJAndAmount = texture2D(linkFirstIndicesAndAmount, index);
  float iCount = linkFirstIJAndAmount.r;
  float jCount = linkFirstIJAndAmount.g;
  float linkAmount = linkFirstIJAndAmount.b;
  if (linkAmount > 0.0) {
    for (float i = 0.0; i < MAX_LINKS; i += 1.0) {
      if (i < linkAmount) {
        if (iCount >= linksTextureSize) {
          iCount = 0.0;
          jCount += 1.0;
        }
        vec2 linkTextureIndex = (vec2(iCount, jCount) + 0.5) / linksTextureSize;
        vec4 connectedPointIndex = texture2D(linkIndices, linkTextureIndex);
        vec4 biasAndStrength = texture2D(linkBiasAndStrength, linkTextureIndex);
        vec4 randomMinDistance = texture2D(linkRandomDistanceFbo, linkTextureIndex);
        float bias = biasAndStrength.r;
        float strength = biasAndStrength.g;
        float randomMinLinkDist = randomMinDistance.r * (linkDistRandomVariationRange.g - linkDistRandomVariationRange.r) + linkDistRandomVariationRange.r;
        randomMinLinkDist *= linkDistance;

        iCount += 1.0;

        vec4 connectedPointPosition = texture2D(position, (connectedPointIndex.rg + 0.5) / pointsTextureSize);
        float x = connectedPointPosition.x - (pointPosition.x + velocity.x);
        float y = connectedPointPosition.y - (pointPosition.y + velocity.y);
        float l = sqrt(x * x + y * y);
        l = max(l, randomMinLinkDist * 0.99);
        l = (l - randomMinLinkDist) / l;
        l *= linkSpring * alpha;
        l *= strength;
        l *= bias;
        x *= l;
        y *= l;
        velocity.x += x;
        velocity.y += y;
      }
    }
  }

  gl_FragColor = vec4(velocity.rg, 0.0, 0.0);
}
  `, maxLinks)
}

// quadtreeFrag generates the Barnes–Hut quadtree force shader, mirroring
// the recursive string generation in quadtree-frag-shader.ts.
func quadtreeFrag(startLevel, maxLevels int) string {
	if startLevel > maxLevels {
		startLevel = maxLevels
	}
	delta := maxLevels - startLevel
	calcAdd := `
    float dist = sqrt(l);
    if (dist > 0.0) {
      float c = alpha * repulsion * centermass.b;
      addVelocity += calcAdd(vec2(x, y), l, c);
      addVelocity += addVelocity * random.rg;
    }
  `
	var quad func(level int) string
	quad = func(level int) string {
		if level >= maxLevels {
			return calcAdd
		}
		groupSize := 1 << uint(level+1)
		var iParts, jParts []string
		for l := 0; l < level+1-delta; l++ {
			iParts = append(iParts, fmt.Sprintf("pow(2.0, %d.0) * i%d", level-(l+delta), l+delta))
			jParts = append(jParts, fmt.Sprintf("pow(2.0, %d.0) * j%d", level-(l+delta), l+delta))
		}
		iEnding := strings.Join(iParts, "+")
		jEnding := strings.Join(jParts, "+")

		return fmt.Sprintf(`
      for (float ij%[1]d = 0.0; ij%[1]d < 4.0; ij%[1]d += 1.0) {
        float i%[1]d = 0.0;
        float j%[1]d = 0.0;
        if (ij%[1]d == 1.0 || ij%[1]d == 3.0) i%[1]d = 1.0;
        if (ij%[1]d == 2.0 || ij%[1]d == 3.0) j%[1]d = 1.0;
        float i = pow(2.0, %[2]d.0) * n / width%[3]d + %[4]s;
        float j = pow(2.0, %[2]d.0) * m / width%[3]d + %[5]s;
        float groupPosX = (i + 0.5) / %[6]d.0;
        float groupPosY = (j + 0.5) / %[6]d.0;

        vec4 centermass = texture2D(level[%[1]d], vec2(groupPosX, groupPosY));
        if (centermass.r > 0.0 && centermass.g > 0.0 && centermass.b > 0.0) {
          float x = centermass.r / centermass.b - pointPosition.r;
          float y = centermass.g / centermass.b - pointPosition.g;
          float l = x * x + y * y;
          if ((width%[3]d * width%[3]d) / theta < l) {
            %[7]s
          } else {
            %[8]s
          }
        }
      }
      `, level, startLevel, level+1, iEnding, jEnding, groupSize, calcAdd, quad(level+1))
	}

	var widths strings.Builder
	for i := 0; i < maxLevels; i++ {
		fmt.Fprintf(&widths, "float width%d = width%d / 2.0;\n", i+1, i)
	}

	return fmt.Sprintf(`
#ifdef GL_ES
precision highp float;
#endif

uniform sampler2D position;
uniform sampler2D randomValues;
uniform float spaceSize;
uniform float repulsion;
uniform float theta;
uniform float alpha;
uniform sampler2D level[%d];
varying vec2 index;

vec2 calcAdd(vec2 xy, float l, float c) {
  float distanceMin2 = 1.0;
  if (l < distanceMin2) l = sqrt(distanceMin2 * l);
  float add = c / l;
  return add * xy;
}

void main() {
  vec4 pointPosition = texture2D(position, index);
  vec4 random = texture2D(randomValues, index);

  float width0 = spaceSize;

  vec2 velocity = vec2(0.0);
  vec2 addVelocity = vec2(0.0);

  %s

  for (float n = 0.0; n < pow(2.0, %d.0); n += 1.0) {
    for (float m = 0.0; m < pow(2.0, %d.0); m += 1.0) {
      %s
    }
  }

  velocity -= addVelocity;

  gl_FragColor = vec4(velocity, 0.0, 0.0);
}
`, maxLevels, widths.String(), delta, delta, quad(delta))
}
