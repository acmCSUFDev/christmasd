import { Point, RGB } from "./christmasd.ts";

export class TreeCanvas {
  private canvas: CanvasRenderingContext2D;
  private canvasWidth: number;
  private canvasHeight: number;

  private colors: RGB[];
  private ledPoints: Point[];
  private ledScale: number;

  constructor(
    canvas: HTMLCanvasElement,
    ledPoints: Point[],
  ) {
    this.canvas = canvas.getContext("2d");
    this.canvasWidth = canvas.width;
    this.canvasHeight = canvas.height;

    const ledMinX = Math.min(...ledPoints.map((p) => p.X));
    const ledMaxX = Math.max(...ledPoints.map((p) => p.X));
    const ledMinY = Math.min(...ledPoints.map((p) => p.Y));
    const ledMaxY = Math.max(...ledPoints.map((p) => p.Y));
    const ledWidth = ledMaxX - ledMinX;
    const ledHeight = ledMaxY - ledMinY;

    this.ledScale = Math.min(this.canvasWidth / ledWidth, this.canvasHeight / ledHeight);

    const ledOffsetX = (this.canvasWidth - ledWidth * this.ledScale) / 2;
    const ledOffsetY = (this.canvasHeight - ledHeight * this.ledScale) / 2;

    this.ledPoints = ledPoints.map((p) => ({
      X: (p.X - ledMinX) * this.ledScale + ledOffsetX,
      Y: (p.Y - ledMinY) * this.ledScale + ledOffsetY,
    }));

    this.clear();
  }

  clear() {
    this.colors = Array(this.ledPoints.length).fill("#FFFFFF");
    this.redraw();
  }

  draw(colors: RGB[]) {
    this.colors = colors;
    this.redraw();
  }

  private redraw() {
    this.canvas.clearRect(0, 0, this.canvasWidth, this.canvasHeight);

    for (const [i, color] of this.colors.entries()) {
      const { X, Y } = this.ledPoints[i];

      this.canvas.beginPath();
      this.canvas.arc(X, Y, 2, 0, 2 * Math.PI);
      this.canvas.fillStyle = color;
      this.canvas.fill();
    }
  }
}
